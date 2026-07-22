package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/knock"
	"mikrotik-psk-knock/client/internal/routeros"
	"mikrotik-psk-knock/client/internal/token"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return flag.ErrHelp
	}
	switch args[0] {
	case "secret":
		return secretCmd(args[1:])
	case "token":
		return tokenCmd(args[1:])
	case "knock":
		return knockCmd(args[1:])
	case "routeros":
		return routerosCmd(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  mkpk secret generate [--bytes 32]
  mkpk token --config mkpk.yaml --client laptop [--bucket N] [--debug]
  mkpk knock --config mkpk.yaml --client laptop [--router host] [--debug] [--min-bucket-age 2s]
  mkpk routeros render --config mkpk.yaml --client laptop [--out generated.rsc]
`)
}

func secretCmd(args []string) error {
	if len(args) == 0 || args[0] != "generate" {
		return fmt.Errorf("usage: mkpk secret generate [--bytes 32]")
	}
	fs := flag.NewFlagSet("secret generate", flag.ContinueOnError)
	n := fs.Int("bytes", 32, "random byte count")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *n < 16 {
		return fmt.Errorf("--bytes must be at least 16")
	}
	buf := make([]byte, *n)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	fmt.Println(base64.RawURLEncoding.EncodeToString(buf))
	return nil
}

func tokenCmd(args []string) error {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	clientName := fs.String("client", "", "client name")
	bucketFlag := fs.String("bucket", "", "override time bucket")
	debug := fs.Bool("debug", false, "print token metadata")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := loadResolved(*configPath, *clientName)
	if err != nil {
		return err
	}
	window := token.InspectWindow(time.Now(), res.Config.Defaults.BucketSeconds)
	bucket := window.Bucket
	if *bucketFlag != "" {
		bucket, err = strconv.ParseInt(*bucketFlag, 10, 64)
		if err != nil {
			return fmt.Errorf("--bucket: %w", err)
		}
	}
	value := token.Compute(res.Client.PSK, res.Service.ServiceName, res.Client.ClientID, bucket)
	if *debug {
		fmt.Printf("service=%s client_id=%s bucket=%d token=%s\n", res.Service.ServiceName, res.Client.ClientID, bucket, value)
		printWindowDebug(window)
		return nil
	}
	fmt.Println(value)
	return nil
}

func knockCmd(args []string) error {
	fs := flag.NewFlagSet("knock", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	clientName := fs.String("client", "", "client name")
	routerAddr := fs.String("router", "", "router address override")
	timeout := fs.Duration("timeout", time.Second, "UDP write timeout")
	interval := fs.Duration("interval", 250*time.Millisecond, "retry interval inside each phase")
	stageDuration := fs.Duration("stage-duration", 2*time.Second, "stage1/stage2 retry duration")
	tokenDuration := fs.Duration("token-duration", time.Second, "token retry duration")
	noisePackets := fs.Int("noise", 0, "random UDP noise packets to send to token port around phases")
	minBucketAge := fs.Duration("min-bucket-age", 2*time.Second, "wait until current bucket is at least this old before sending token")
	debug := fs.Bool("debug", false, "print knock metadata")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := loadResolved(*configPath, *clientName)
	if err != nil {
		return err
	}
	router := *routerAddr
	if router == "" {
		router = res.Config.Router.Address
	}
	now, err := waitForBucketAge(res.Config.Defaults.BucketSeconds, *minBucketAge, *debug)
	if err != nil {
		return err
	}
	window := token.InspectWindow(now, res.Config.Defaults.BucketSeconds)
	bucket := window.Bucket
	value := token.Compute(res.Client.PSK, res.Service.ServiceName, res.Client.ClientID, bucket)
	if *debug {
		fmt.Printf("router=%s service=%s client_id=%s bucket=%d stage1=%d stage2=%d token_port=%d interval=%s stage_duration=%s token_duration=%s noise=%d min_bucket_age=%s\n",
			router, res.Service.ServiceName, res.Client.ClientID, bucket, res.Service.Stage1Port, res.Service.Stage2Port, res.Service.TokenPort, *interval, *stageDuration, *tokenDuration, *noisePackets, *minBucketAge)
		printWindowDebug(window)
	}
	var logf func(string, ...any)
	if *debug {
		logf = func(format string, args ...any) {
			fmt.Printf(format+"\n", args...)
		}
	}
	return knock.Run(knock.Options{
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
	})
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

func routerosCmd(args []string) error {
	if len(args) == 0 || args[0] != "render" {
		return fmt.Errorf("usage: mkpk routeros render --config mkpk.yaml --client laptop [--out generated.rsc]")
	}
	fs := flag.NewFlagSet("routeros render", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	clientName := fs.String("client", "", "client name")
	outPath := fs.String("out", "", "output .rsc path; stdout when empty")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	res, err := loadResolved(*configPath, *clientName)
	if err != nil {
		return err
	}
	rendered, err := routeros.Render(res)
	if err != nil {
		return err
	}
	if *outPath == "" {
		fmt.Print(rendered)
		return nil
	}
	return os.WriteFile(*outPath, []byte(rendered), 0600)
}

func loadResolved(path, clientName string) (config.Resolved, error) {
	if clientName == "" {
		return config.Resolved{}, fmt.Errorf("--client is required")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Resolved{}, err
	}
	return cfg.Resolve(clientName)
}
