package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
	"mikrotik-psk-knock/client/internal/config"
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
	case "config":
		return configCmd(args[1:])
	case "profile":
		return profileCmd(args[1:])
	case "client":
		return clientCmd(args[1:])
	case "token":
		return tokenCmd(args[1:])
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
  mkpk-provision secret generate [--bytes 32]
  mkpk-provision config validate --config mkpk.yaml
  mkpk-provision profile init --out mkpk.yaml --router-address host
  mkpk-provision client add --config mkpk.yaml --name laptop --service service
  mkpk-provision token --config mkpk.yaml --client laptop [--bucket N] [--debug]
  mkpk-provision routeros render --config mkpk.yaml --client laptop [--out generated.rsc]
`)
}

func secretCmd(args []string) error {
	if len(args) == 0 || args[0] != "generate" {
		return fmt.Errorf("usage: mkpk-provision secret generate [--bytes 32]")
	}
	fs := flag.NewFlagSet("secret generate", flag.ContinueOnError)
	n := fs.Int("bytes", 32, "random byte count")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *n < 16 {
		return fmt.Errorf("--bytes must be at least 16")
	}
	secret, err := generateSecret(*n)
	if err != nil {
		return err
	}
	fmt.Println(secret)
	return nil
}

func generateSecret(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func configCmd(args []string) error {
	if len(args) == 0 || args[0] != "validate" {
		return fmt.Errorf("usage: mkpk-provision config validate --config mkpk.yaml")
	}
	fs := flag.NewFlagSet("config validate", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	printConfigSummary(cfg, *configPath)
	return nil
}

func profileCmd(args []string) error {
	if len(args) == 0 || args[0] != "init" {
		return fmt.Errorf("usage: mkpk-provision profile init --out mkpk.yaml --router-address host")
	}
	fs := flag.NewFlagSet("profile init", flag.ContinueOnError)
	outPath := fs.String("out", "mkpk.yaml", "output config path")
	routerName := fs.String("router-name", "mikrotik", "RouterOS identity label")
	routerAddress := fs.String("router-address", "", "RouterOS public address")
	serviceName := fs.String("service", "demo-service", "initial service name")
	clientName := fs.String("client", "demo-client", "initial client name")
	force := fs.Bool("force", false, "overwrite output file when it exists")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *routerAddress == "" {
		return fmt.Errorf("--router-address is required")
	}
	if *serviceName == "" {
		return fmt.Errorf("--service is required")
	}
	if *clientName == "" {
		return fmt.Errorf("--client is required")
	}
	if !*force {
		if _, err := os.Stat(*outPath); err == nil {
			return fmt.Errorf("%s already exists; use --force to overwrite", *outPath)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	psk, err := generateSecret(32)
	if err != nil {
		return err
	}
	cfg := initialConfig(*routerName, *routerAddress, *serviceName, *clientName, psk)
	if err := writeConfig(*outPath, cfg); err != nil {
		return err
	}
	fmt.Printf("created %s service=%s client=%s\n", *outPath, *serviceName, *clientName)
	return nil
}

func initialConfig(routerName, routerAddress, serviceName, clientName, psk string) config.Config {
	return config.Config{
		Router: config.Router{
			Name:    routerName,
			Address: routerAddress,
		},
		Defaults: config.Defaults{
			BucketSeconds:   30,
			StageTimeout:    "5s",
			TokenHitTimeout: "2s",
			AllowedTimeout:  "3m",
			UsedTimeout:     "65s",
		},
		Services: map[string]config.Service{
			serviceName: {
				ServiceName: serviceName,
				Stage1Port:  41001,
				Stage2Port:  41002,
				TokenPort:   41003,
				AllowedList: "mkpk-tt-allowed",
				NAT: config.NAT{
					Enabled:   false,
					Comment:   "mkpk-tt dst-nat " + serviceName,
					DstPort:   2222,
					ToAddress: "192.0.2.10",
					ToPort:    22,
				},
				Notify: config.Notify{
					Enabled: false,
					URL:     "",
				},
			},
		},
		Clients: map[string]config.Client{
			clientName: {
				ClientID: clientName,
				Service:  serviceName,
				PSK:      psk,
			},
		},
	}
}

func clientCmd(args []string) error {
	if len(args) == 0 || args[0] != "add" {
		return fmt.Errorf("usage: mkpk-provision client add --config mkpk.yaml --name laptop --service service")
	}
	fs := flag.NewFlagSet("client add", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	name := fs.String("name", "", "client map key")
	clientID := fs.String("client-id", "", "client_id; --name when empty")
	serviceName := fs.String("service", "", "service name")
	pskFlag := fs.String("psk", "", "client PSK; generated when empty")
	force := fs.Bool("force", false, "replace existing client")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *serviceName == "" {
		return fmt.Errorf("--service is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if _, ok := cfg.Services[*serviceName]; !ok {
		return fmt.Errorf("unknown service %q", *serviceName)
	}
	if _, ok := cfg.Clients[*name]; ok && !*force {
		return fmt.Errorf("client %q already exists; use --force to replace", *name)
	}
	id := *clientID
	if id == "" {
		id = *name
	}
	psk := *pskFlag
	pskSource := "provided"
	if psk == "" {
		psk, err = generateSecret(32)
		if err != nil {
			return err
		}
		pskSource = "generated"
	}
	cfg.Clients[*name] = config.Client{
		ClientID: id,
		Service:  *serviceName,
		PSK:      psk,
	}
	if err := writeConfig(*configPath, cfg); err != nil {
		return err
	}
	fmt.Printf("client added config=%s name=%s client_id=%s service=%s psk=%s\n", *configPath, *name, id, *serviceName, pskSource)
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

func routerosCmd(args []string) error {
	if len(args) == 0 || args[0] != "render" {
		return fmt.Errorf("usage: mkpk-provision routeros render --config mkpk.yaml --client laptop [--out generated.rsc]")
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

func writeConfig(path string, cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func printConfigSummary(cfg config.Config, path string) {
	fmt.Printf("config=%s status=valid\n", path)
	fmt.Printf("router name=%s address=%s\n", cfg.Router.Name, cfg.Router.Address)
	fmt.Printf("defaults bucket_seconds=%d stage_timeout=%s token_hit_timeout=%s allowed_timeout=%s used_timeout=%s\n",
		cfg.Defaults.BucketSeconds,
		cfg.Defaults.StageTimeout,
		cfg.Defaults.TokenHitTimeout,
		cfg.Defaults.AllowedTimeout,
		cfg.Defaults.UsedTimeout,
	)
	serviceNames := sortedServiceNames(cfg.Services)
	clientNames := sortedClientNames(cfg.Clients)
	fmt.Printf("services count=%d names=%s\n", len(serviceNames), joinNames(serviceNames))
	for _, name := range serviceNames {
		svc := cfg.Services[name]
		fmt.Printf("service name=%s service_name=%s stage1=%d stage2=%d token=%d allowed_list=%s nat_enabled=%t nat_dst_port=%d nat_to=%s:%d notify_enabled=%t\n",
			name,
			svc.ServiceName,
			svc.Stage1Port,
			svc.Stage2Port,
			svc.TokenPort,
			svc.AllowedList,
			svc.NAT.Enabled,
			svc.NAT.DstPort,
			svc.NAT.ToAddress,
			svc.NAT.ToPort,
			svc.Notify.Enabled,
		)
	}
	fmt.Printf("clients count=%d names=%s\n", len(clientNames), joinNames(clientNames))
	for _, name := range clientNames {
		client := cfg.Clients[name]
		fmt.Printf("client name=%s client_id=%s service=%s psk=set\n", name, client.ClientID, client.Service)
	}
}

func sortedServiceNames(services map[string]config.Service) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedClientNames(clients map[string]config.Client) []string {
	names := make([]string, 0, len(clients))
	for name := range clients {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "-"
	}
	out := names[0]
	for _, name := range names[1:] {
		out += "," + name
	}
	return out
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
