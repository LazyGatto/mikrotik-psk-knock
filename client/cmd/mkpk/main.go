package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/invite"
	"mikrotik-psk-knock/client/internal/knock"
	"mikrotik-psk-knock/client/internal/servicecheck"
	"mikrotik-psk-knock/client/internal/token"
	"mikrotik-psk-knock/client/internal/version"
)

func main() {
	err := run(os.Args[1:])
	if err == nil {
		return
	}
	// `-h`/`--help` surfaces as flag.ErrHelp after the flag package already
	// printed the usage — it's not a real error, so don't tack on "error: …".
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if !isSilentError(err) {
		fmt.Fprintln(os.Stderr, "error:", err)
		if cmd := subcommand(os.Args[1:]); cmd != "" {
			fmt.Fprintf(os.Stderr, "run `mkpk %s -h` for all options\n", cmd)
		} else {
			fmt.Fprintln(os.Stderr, "run `mkpk help` for usage")
		}
	}
	os.Exit(1)
}

// subcommand returns knock/check when that's what was invoked, so an error can
// point at the right `-h`.
func subcommand(args []string) string {
	if len(args) > 0 && (args[0] == "knock" || args[0] == "check" || args[0] == "invite") {
		return args[0]
	}
	return ""
}

type silentError struct {
	err error
}

func (e silentError) Error() string {
	return e.err.Error()
}

func isSilentError(err error) bool {
	_, ok := err.(silentError)
	return ok
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return flag.ErrHelp
	}
	switch args[0] {
	case "knock":
		return knockCmd(args[1:])
	case "check":
		return checkCmd(args[1:])
	case "invite":
		return inviteCmd(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("mkpk %s\n", version.String())
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  mkpk knock (--invite @laptop.mkpk | --config mkpk.yaml --client laptop [--router name]) [--service name] [--check] [--debug]
  mkpk check (--invite @laptop.mkpk | --config mkpk.yaml --client laptop [--router name]) [--service name] [--host host] [--port port] [--json] [--debug]
  mkpk invite show @laptop.mkpk [--json]

Run `+"`mkpk knock -h`"+` or `+"`mkpk check -h`"+` for all options (e.g. --noise, --min-bucket-age, --stage-duration).
`)
}

func knockCmd(args []string) error {
	fs := flag.NewFlagSet("knock", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	clientName := fs.String("client", "", "client name")
	routerName := fs.String("router", "", "router name; sole router when empty")
	serviceName := fs.String("service", "", "service name; sole service when empty")
	routerAddr := fs.String("address", "", "router address override")
	inviteFlag := fs.String("invite", "", "invite blob (base64) or @path; overrides --config/--client")
	timeout := fs.Duration("timeout", time.Second, "UDP write timeout")
	interval := fs.Duration("interval", 250*time.Millisecond, "retry interval inside each phase")
	stageDuration := fs.Duration("stage-duration", 2*time.Second, "stage1/stage2 retry duration")
	tokenDuration := fs.Duration("token-duration", time.Second, "token retry duration")
	noisePackets := fs.Int("noise", 0, "random UDP noise packets to send to token port around phases")
	minBucketAge := fs.Duration("min-bucket-age", 2*time.Second, "wait until current bucket is at least this old before sending token")
	check := fs.Bool("check", false, "check target TCP port after knock")
	checkHost := fs.String("check-host", "", "target host for post-knock TCP check; router address when empty")
	checkPort := fs.Int("check-port", 0, "target TCP port for post-knock check; service nat.dst_port when empty")
	checkTimeout := fs.Duration("check-timeout", time.Second, "per-attempt TCP check timeout")
	checkAttempts := fs.Int("check-attempts", 10, "TCP check attempts after knock")
	checkInterval := fs.Duration("check-interval", 500*time.Millisecond, "delay between TCP check attempts")
	debug := fs.Bool("debug", false, "print knock metadata")
	jsonOut := fs.Bool("json", false, "print the result as JSON (includes the check when --check)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := resolveTarget(*configPath, *inviteFlag, *routerName, *clientName, *serviceName)
	if err != nil {
		return err
	}
	router := *routerAddr
	if router == "" {
		router = res.Router.Address
	}
	now, err := waitForBucketAge(res.Router.Defaults.BucketSeconds, *minBucketAge, *debug)
	if err != nil {
		return err
	}
	window := token.InspectWindow(now, res.Router.Defaults.BucketSeconds)
	bucket := window.Bucket
	value := token.Compute(res.PSK, res.Service.ServiceName, res.ClientID, bucket)
	if *debug {
		fmt.Printf("router=%s service=%s client_id=%s bucket=%d stage1=%d stage2=%d token_port=%d interval=%s stage_duration=%s token_duration=%s noise=%d min_bucket_age=%s check=%t\n",
			router, res.Service.ServiceName, res.ClientID, bucket, res.Service.Stage1Port, res.Service.Stage2Port, res.Service.TokenPort, *interval, *stageDuration, *tokenDuration, *noisePackets, *minBucketAge, *check)
		printWindowDebug(window)
	}
	var logf func(string, ...any)
	if *debug {
		logf = func(format string, args ...any) {
			fmt.Printf(format+"\n", args...)
		}
	}
	if err := knock.Run(knock.Options{
		Router:        router,
		Stage1Port:    res.Service.Stage1Port,
		Stage2Port:    res.Service.Stage2Port,
		TokenPort:     res.Service.TokenPort,
		Token:         value,
		Timeout:       *timeout,
		Interval:      *interval,
		StageDuration: *stageDuration,
		TokenDuration: *tokenDuration,
		NoisePackets:  *noisePackets,
		Logf:          logf,
	}); err != nil {
		return err
	}
	// Optionally verify the target port opened. We always compute a Result (not
	// servicecheck.Run) so the outcome can be reported in both JSON and text.
	var chk *servicecheck.Result
	var host string
	var port int
	if *check {
		host, port = resolveCheckTarget(res, router, *checkHost, *checkPort)
		if *debug {
			fmt.Printf("check_host=%s check_port=%d check_timeout=%s check_attempts=%d check_interval=%s\n",
				host, port, *checkTimeout, *checkAttempts, *checkInterval)
		}
		r := servicecheck.Check(servicecheck.Options{
			Host: host, Port: port, Timeout: *checkTimeout, Attempts: *checkAttempts, Interval: *checkInterval, Logf: logf,
		})
		chk = &r
	}

	if *jsonOut {
		out := map[string]any{
			"router": router, "service": res.Service.ServiceName,
			"client_id": res.ClientID, "bucket": bucket, "knocked": true,
		}
		if chk != nil {
			out["check"] = map[string]any{
				"status": chk.Status, "host": host, "port": port,
				"attempts": chk.Attempts, "duration_ms": chk.Duration.Milliseconds(),
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
	} else {
		// A concise result line so success isn't silent (useful in Ansible etc.).
		line := fmt.Sprintf("knock sent: router=%s service=%s client=%s bucket=%d",
			router, res.Service.ServiceName, res.ClientID, bucket)
		if chk != nil {
			line += fmt.Sprintf("; check %s (port %d, %dms)", chk.Status, port, chk.Duration.Milliseconds())
		}
		fmt.Println(line)
	}

	// Non-zero exit when the check ran and the port isn't open (output already
	// printed, so don't add an "error:" line on top).
	if chk != nil && chk.Status != "open" {
		return silentError{fmt.Errorf("port %s", chk.Status)}
	}
	return nil
}

func checkCmd(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	clientName := fs.String("client", "", "client name")
	routerName := fs.String("router", "", "router name; sole router when empty")
	serviceName := fs.String("service", "", "service name; sole service when empty")
	routerAddr := fs.String("address", "", "router address override")
	inviteFlag := fs.String("invite", "", "invite blob (base64) or @path; overrides --config/--client")
	hostFlag := fs.String("host", "", "target host override; router address when empty")
	portFlag := fs.Int("port", 0, "target TCP port override; service nat.dst_port when empty")
	timeout := fs.Duration("timeout", time.Second, "per-attempt TCP check timeout")
	attempts := fs.Int("attempts", 1, "TCP check attempts")
	interval := fs.Duration("interval", 500*time.Millisecond, "delay between TCP check attempts")
	debug := fs.Bool("debug", false, "print check metadata")
	jsonOutput := fs.Bool("json", false, "print machine-readable JSON result")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := resolveTarget(*configPath, *inviteFlag, *routerName, *clientName, *serviceName)
	if err != nil {
		return err
	}
	router := *routerAddr
	if router == "" {
		router = res.Router.Address
	}
	host, port := resolveCheckTarget(res, router, *hostFlag, *portFlag)
	var logf func(string, ...any)
	if *debug && !*jsonOutput {
		fmt.Printf("check_host=%s check_port=%d check_timeout=%s check_attempts=%d check_interval=%s\n",
			host, port, *timeout, *attempts, *interval)
		logf = func(format string, args ...any) {
			fmt.Printf(format+"\n", args...)
		}
	}
	result := servicecheck.Check(servicecheck.Options{
		Host:     host,
		Port:     port,
		Timeout:  *timeout,
		Attempts: *attempts,
		Interval: *interval,
		Logf:     logf,
	})
	if *jsonOutput {
		if err := printCheckJSON(result); err != nil {
			return err
		}
	} else {
		// Don't stay silent on success — print a concise result line.
		line := fmt.Sprintf("check %s: %s:%d (%dms)", result.Status, host, port, result.Duration.Milliseconds())
		if result.Status != "open" && result.LastError != "" {
			line += " — " + result.LastError
		}
		fmt.Println(line)
	}
	if result.Status != "open" {
		// Output already printed above, so don't add an "error:" line on top.
		return silentError{err: fmt.Errorf("%s", result.LastError)}
	}
	return nil
}

type checkJSON struct {
	Status     string `json:"status"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Attempts   int    `json:"attempts"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

func printCheckJSON(result servicecheck.Result) error {
	payload := checkJSON{
		Status:     result.Status,
		Host:       result.Host,
		Port:       result.Port,
		Attempts:   result.Attempts,
		DurationMS: result.Duration.Milliseconds(),
		Error:      result.LastError,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func resolveCheckTarget(res config.Resolved, router, hostOverride string, portOverride int) (string, int) {
	host := hostOverride
	if host == "" {
		host = router
	}
	port := portOverride
	if port == 0 {
		port = res.Service.Target.Port
	}
	return host, port
}

func waitForBucketAge(bucketSeconds int64, minAge time.Duration, debug bool) (time.Time, error) {
	if minAge < 0 {
		return time.Time{}, fmt.Errorf("--min-bucket-age must be non-negative")
	}
	bucketDuration := time.Duration(bucketSeconds) * time.Second
	if minAge >= bucketDuration {
		return time.Time{}, fmt.Errorf("--min-bucket-age must be less than bucket duration %s", bucketDuration)
	}
	now := time.Now()
	window := token.InspectWindow(now, bucketSeconds)
	if window.Age >= minAge {
		return now, nil
	}
	wait := minAge - window.Age
	if debug {
		fmt.Printf("bucket_age=%s min_bucket_age=%s wait=%s\n", window.Age.Truncate(time.Millisecond), minAge, wait.Truncate(time.Millisecond))
	}
	time.Sleep(wait)
	return time.Now(), nil
}

func printWindowDebug(window token.Window) {
	fmt.Printf("local_time=%s unix=%d bucket_seconds=%d accepted_buckets=%d,%d next_bucket=%d bucket_start=%s bucket_end=%s bucket_age=%s bucket_remaining=%s\n",
		window.Time.Format(time.RFC3339),
		window.Time.Unix(),
		window.BucketSeconds,
		window.Previous,
		window.Bucket,
		window.Next,
		window.Start.Format(time.RFC3339),
		window.End.Format(time.RFC3339),
		window.Age.Truncate(time.Millisecond),
		window.Remaining.Truncate(time.Millisecond),
	)
}

// inviteCmd inspects an invite file. It exists because an invite that no longer
// matches the router fails silently — a UDP knock cannot report delivery — so
// the only way to tell "my invite is stale" from "the router is down" is to
// compare what the invite carries against what the admin console shows. The
// fingerprint makes that a one-line comparison.
func inviteCmd(args []string) error {
	if len(args) == 0 || args[0] != "show" {
		return fmt.Errorf("usage: mkpk invite show @laptop.mkpk [--json]")
	}
	fs := flag.NewFlagSet("invite show", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mkpk invite show @laptop.mkpk [--json]")
	}
	b, err := loadBlob(fs.Arg(0))
	if err != nil {
		return err
	}

	// The PSK is never printed, in either format: this output is meant to be
	// pasted into a chat with the admin.
	type svcOut struct {
		Name           string `json:"name"`
		Stage1         int    `json:"stage1"`
		Stage2         int    `json:"stage2"`
		Token          int    `json:"token"`
		CheckPort      int    `json:"check_port"`
		AllowedTimeout string `json:"allowed_timeout,omitempty"`
		Launch         string `json:"launch,omitempty"`
	}
	type routerOut struct {
		Router        string   `json:"router"`
		BucketSeconds int64    `json:"bucket_seconds"`
		Fingerprint   string   `json:"fingerprint"`
		Services      []svcOut `json:"services"`
	}
	out := struct {
		Version     int         `json:"v"`
		ClientID    string      `json:"client_id"`
		Fingerprint string      `json:"fingerprint"`
		Routers     []routerOut `json:"routers"`
	}{Version: b.Version, ClientID: b.ClientID, Fingerprint: invite.Fingerprint(b)}
	for _, r := range b.Routers {
		ro := routerOut{Router: r.Router, BucketSeconds: r.BucketSeconds,
			Fingerprint: invite.RouterFingerprint(b.ClientID, r)}
		for _, s := range r.Services {
			ro.Services = append(ro.Services, svcOut{
				Name: s.Name, Stage1: s.Stage1, Stage2: s.Stage2, Token: s.Token,
				CheckPort: s.CheckPort, AllowedTimeout: s.AllowedTimeout, Launch: s.Launch,
			})
		}
		out.Routers = append(out.Routers, ro)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("client_id:   %s\n", out.ClientID)
	fmt.Printf("fingerprint: %s\n", out.Fingerprint)
	for _, r := range out.Routers {
		fmt.Printf("\nrouter %s (bucket %ds, fingerprint %s)\n", r.Router, r.BucketSeconds, r.Fingerprint)
		for _, s := range r.Services {
			line := fmt.Sprintf("  %-16s knock %d/%d/%d  check %d", s.Name, s.Stage1, s.Stage2, s.Token, s.CheckPort)
			if s.AllowedTimeout != "" {
				line += "  opens for " + s.AllowedTimeout
			}
			if s.Launch != "" {
				line += "  launch " + s.Launch
			}
			fmt.Println(line)
		}
	}
	fmt.Printf("\nCompare the fingerprint with the one your admin sees next to your access.\n" +
		"If they differ, this invite is stale — ask for a new one.\n")
	return nil
}

func resolveTarget(configPath, inviteFlag, routerName, clientName, serviceName string) (config.Resolved, error) {
	if inviteFlag != "" {
		b, err := loadBlob(inviteFlag)
		if err != nil {
			return config.Resolved{}, err
		}
		cfg := b.ToConfig()
		rn, err := pickRouterKey(cfg, routerName)
		if err != nil {
			return config.Resolved{}, err
		}
		return cfg.Resolve(b.ClientID, rn, serviceName)
	}
	return loadResolved(configPath, routerName, clientName, serviceName)
}

// pickRouterKey resolves the router selector against a config's router keys.
// An empty selector is allowed only when there is exactly one router.
func pickRouterKey(cfg config.Config, routerName string) (string, error) {
	if routerName != "" {
		if _, ok := cfg.Routers[routerName]; !ok {
			return "", fmt.Errorf("unknown router %q (have %v)", routerName, routerKeys(cfg))
		}
		return routerName, nil
	}
	if len(cfg.Routers) != 1 {
		return "", fmt.Errorf("--router is required (invite has %d routers: %v)", len(cfg.Routers), routerKeys(cfg))
	}
	for name := range cfg.Routers {
		return name, nil
	}
	return "", fmt.Errorf("no routers")
}

func routerKeys(cfg config.Config) []string {
	keys := make([]string, 0, len(cfg.Routers))
	for k := range cfg.Routers {
		keys = append(keys, k)
	}
	return keys
}

func loadBlob(v string) (invite.Blob, error) {
	s := v
	if path, ok := strings.CutPrefix(v, "@"); ok {
		// Explicit file reference.
		data, err := os.ReadFile(path)
		if err != nil {
			return invite.Blob{}, err
		}
		s = strings.TrimSpace(string(data))
	} else if info, err := os.Stat(v); err == nil && !info.IsDir() {
		// Bare path to an existing file — convenience, so `--invite laptop.mkpk`
		// works without the @ prefix. A real blob string won't match a file.
		data, err := os.ReadFile(v)
		if err != nil {
			return invite.Blob{}, err
		}
		s = strings.TrimSpace(string(data))
	}
	return invite.Decode(s)
}

func loadResolved(path, routerName, clientName, serviceName string) (config.Resolved, error) {
	if clientName == "" {
		return config.Resolved{}, fmt.Errorf("--client is required")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Resolved{}, err
	}
	rn, err := pickRouterKey(cfg, routerName)
	if err != nil {
		return config.Resolved{}, err
	}
	return cfg.Resolve(clientName, rn, serviceName)
}
