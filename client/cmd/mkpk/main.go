package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/knock"
	"mikrotik-psk-knock/client/internal/servicecheck"
	"mikrotik-psk-knock/client/internal/token"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if !isSilentError(err) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
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
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  mkpk check --config mkpk.yaml --client laptop [--router name] [--service name] [--host host] [--port port] [--json] [--debug]
  mkpk knock --config mkpk.yaml --client laptop [--router name] [--service name] [--address host] [--check] [--debug]
`)
}

func knockCmd(args []string) error {
	fs := flag.NewFlagSet("knock", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	clientName := fs.String("client", "", "client name")
	routerName := fs.String("router", "", "router name; sole router when empty")
	serviceName := fs.String("service", "", "service name; sole service when empty")
	routerAddr := fs.String("address", "", "router address override")
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := loadResolved(*configPath, *routerName, *clientName, *serviceName)
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
	value := token.Compute(res.Client.PSK, res.Service.ServiceName, res.Client.ClientID, bucket)
	if *debug {
		fmt.Printf("router=%s service=%s client_id=%s bucket=%d stage1=%d stage2=%d token_port=%d interval=%s stage_duration=%s token_duration=%s noise=%d min_bucket_age=%s check=%t\n",
			router, res.Service.ServiceName, res.Client.ClientID, bucket, res.Service.Stage1Port, res.Service.Stage2Port, res.Service.TokenPort, *interval, *stageDuration, *tokenDuration, *noisePackets, *minBucketAge, *check)
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
	if !*check {
		return nil
	}
	host, port := resolveCheckTarget(res, router, *checkHost, *checkPort)
	if *debug {
		fmt.Printf("check_host=%s check_port=%d check_timeout=%s check_attempts=%d check_interval=%s\n",
			host, port, *checkTimeout, *checkAttempts, *checkInterval)
	}
	return servicecheck.Run(servicecheck.Options{
		Host:     host,
		Port:     port,
		Timeout:  *checkTimeout,
		Attempts: *checkAttempts,
		Interval: *checkInterval,
		Logf:     logf,
	})
}

func checkCmd(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	clientName := fs.String("client", "", "client name")
	routerName := fs.String("router", "", "router name; sole router when empty")
	serviceName := fs.String("service", "", "service name; sole service when empty")
	routerAddr := fs.String("address", "", "router address override")
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
	res, err := loadResolved(*configPath, *routerName, *clientName, *serviceName)
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
	}
	if result.Status == "open" {
		return nil
	}
	if *jsonOutput {
		return silentError{err: fmt.Errorf("%s", result.LastError)}
	}
	return fmt.Errorf("%s", result.LastError)
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
		port = res.Service.NAT.DstPort
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

func loadResolved(path, routerName, clientName, serviceName string) (config.Resolved, error) {
	if clientName == "" {
		return config.Resolved{}, fmt.Errorf("--client is required")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Resolved{}, err
	}
	if routerName == "" {
		if len(cfg.Routers) != 1 {
			return config.Resolved{}, fmt.Errorf("--router is required (config has %d routers)", len(cfg.Routers))
		}
		for name := range cfg.Routers {
			routerName = name
		}
	}
	r, ok := cfg.Router(routerName)
	if !ok {
		return config.Resolved{}, fmt.Errorf("unknown router %q", routerName)
	}
	return r.Resolve(routerName, clientName, serviceName)
}
