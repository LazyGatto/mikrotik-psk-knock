package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"mikrotik-psk-knock/client/internal/admin"
	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/deploy"
	"mikrotik-psk-knock/client/internal/token"
	"mikrotik-psk-knock/client/internal/web"
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
	case "service":
		return serviceCmd(args[1:])
	case "token":
		return tokenCmd(args[1:])
	case "routeros":
		return routerosCmd(args[1:])
	case "deploy":
		return deployCmd(args[1:])
	case "serve":
		return serveCmd(args[1:])
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
  mkpk-provision service add --config mkpk.yaml --name ssh --stage1-port 41011 --stage2-port 41012 --token-port 41013 --nat-dst-port 2022 --nat-to-address 192.0.2.10 --nat-to-port 22
  mkpk-provision client add --config mkpk.yaml --name laptop --service service
  mkpk-provision token --config mkpk.yaml --client laptop [--bucket N] [--debug]
  mkpk-provision routeros render --config mkpk.yaml [--client laptop] [--out generated.rsc]
  mkpk-provision deploy [status|uninstall] --config mkpk.yaml --user admin [--key ~/.ssh/id_ed25519] [--address host]
  mkpk-provision serve --config mkpk.yaml [--addr 127.0.0.1:8765]
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
	secret, err := admin.GenerateSecret(*n)
	if err != nil {
		return err
	}
	fmt.Println(secret)
	return nil
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
	printSummary(*configPath, admin.Summarize(cfg))
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
	if !*force {
		if _, err := os.Stat(*outPath); err == nil {
			return fmt.Errorf("%s already exists; use --force to overwrite", *outPath)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	cfg, err := admin.InitConfig(admin.InitOptions{
		RouterName:    *routerName,
		RouterAddress: *routerAddress,
		ServiceName:   *serviceName,
		ClientName:    *clientName,
	})
	if err != nil {
		return err
	}
	if err := admin.SaveConfig(*outPath, cfg); err != nil {
		return err
	}
	fmt.Printf("created %s service=%s client=%s\n", *outPath, *serviceName, *clientName)
	return nil
}

func serviceCmd(args []string) error {
	if len(args) == 0 || args[0] != "add" {
		return fmt.Errorf("usage: mkpk-provision service add --config mkpk.yaml --name ssh --stage1-port 41011 --stage2-port 41012 --token-port 41013 --nat-dst-port 2022 --nat-to-address 192.0.2.10 --nat-to-port 22")
	}
	fs := flag.NewFlagSet("service add", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	name := fs.String("name", "", "service map key")
	serviceName := fs.String("service-name", "", "service_name; --name when empty")
	stage1Port := fs.Int("stage1-port", 0, "stage1 UDP port")
	stage2Port := fs.Int("stage2-port", 0, "stage2 UDP port")
	tokenPort := fs.Int("token-port", 0, "token UDP port")
	allowedList := fs.String("allowed-list", "", "RouterOS allowed address-list; mkpk-tt-allowed-<name> when empty")
	natEnabled := fs.Bool("nat-enabled", false, "enable generated dst-nat rule")
	natComment := fs.String("nat-comment", "", "stable RouterOS NAT rule comment")
	natDstPort := fs.Int("nat-dst-port", 0, "external TCP dst-nat port")
	natToAddress := fs.String("nat-to-address", "", "internal service address")
	natToPort := fs.Int("nat-to-port", 0, "internal service port")
	notifyEnabled := fs.Bool("notify-enabled", false, "enable notification")
	notifyChannel := fs.String("notify-channel", "webhook", "notification channel: webhook, telegram or email")
	notifyURL := fs.String("notify-url", "", "webhook notification URL")
	notifyTgToken := fs.String("notify-telegram-bot-token", "", "telegram bot token")
	notifyTgChat := fs.String("notify-telegram-chat-id", "", "telegram chat id")
	notifyEmailTo := fs.String("notify-email-to", "", "email recipient")
	notifyEmailFrom := fs.String("notify-email-from", "", "email sender")
	notifyEmailServer := fs.String("notify-email-server", "", "SMTP server host")
	notifyEmailPort := fs.Int("notify-email-port", 0, "SMTP port (default 587)")
	notifyEmailTLS := fs.String("notify-email-tls", "", "SMTP tls: no, yes or starttls (default starttls)")
	notifyEmailUser := fs.String("notify-email-user", "", "SMTP username")
	notifyEmailPassword := fs.String("notify-email-password", "", "SMTP password")
	force := fs.Bool("force", false, "replace existing service")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	cfg, err = admin.AddService(cfg, admin.ServiceOptions{
		Name:        *name,
		ServiceName: *serviceName,
		Stage1Port:  *stage1Port,
		Stage2Port:  *stage2Port,
		TokenPort:   *tokenPort,
		AllowedList: *allowedList,
		NAT: config.NAT{
			Enabled:   *natEnabled,
			Comment:   *natComment,
			DstPort:   *natDstPort,
			ToAddress: *natToAddress,
			ToPort:    *natToPort,
		},
		Notify: config.Notify{
			Enabled:  *notifyEnabled,
			Channel:  *notifyChannel,
			URL:      *notifyURL,
			Telegram: config.NotifyTelegram{BotToken: *notifyTgToken, ChatID: *notifyTgChat},
			Email: config.NotifyEmail{
				To:       *notifyEmailTo,
				From:     *notifyEmailFrom,
				Server:   *notifyEmailServer,
				Port:     *notifyEmailPort,
				TLS:      *notifyEmailTLS,
				User:     *notifyEmailUser,
				Password: *notifyEmailPassword,
			},
		},
		Force: *force,
	})
	if err != nil {
		return err
	}
	if err := admin.SaveConfig(*configPath, cfg); err != nil {
		return err
	}
	svc := cfg.Services[*name]
	fmt.Printf("service added config=%s name=%s service_name=%s stage1=%d stage2=%d token=%d nat_enabled=%t nat_dst_port=%d nat_to=%s:%d\n",
		*configPath, *name, svc.ServiceName, svc.Stage1Port, svc.Stage2Port, svc.TokenPort, svc.NAT.Enabled, svc.NAT.DstPort, svc.NAT.ToAddress, svc.NAT.ToPort)
	return nil
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
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	res, err := admin.AddClient(cfg, admin.ClientOptions{
		Name:     *name,
		ClientID: *clientID,
		Service:  *serviceName,
		PSK:      *pskFlag,
		Force:    *force,
	})
	if err != nil {
		return err
	}
	if err := admin.SaveConfig(*configPath, res.Config); err != nil {
		return err
	}
	cl := res.Config.Clients[*name]
	fmt.Printf("client added config=%s name=%s client_id=%s service=%s psk=%s\n", *configPath, *name, cl.ClientID, *serviceName, res.PSKSource)
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
	clientName := fs.String("client", "", "client name; renders all clients when empty")
	outPath := fs.String("out", "", "output .rsc path; stdout when empty")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	rendered, err := admin.Render(cfg, *clientName)
	if err != nil {
		return err
	}
	if *outPath == "" {
		fmt.Print(rendered)
		return nil
	}
	return os.WriteFile(*outPath, []byte(rendered), 0600)
}

func deployCmd(args []string) error {
	sub := "install"
	if len(args) > 0 && (args[0] == "status" || args[0] == "uninstall") {
		sub = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	address := fs.String("address", "", "router address override; router.address when empty")
	user := fs.String("user", "admin", "SSH username")
	port := fs.Int("port", 22, "SSH port")
	keyPath := fs.String("key", "", "SSH private key path")
	keyPass := fs.String("key-pass", "", "passphrase for the SSH key")
	useAgent := fs.Bool("agent", true, "use ssh-agent if available")
	password := fs.String("password", "", "SSH password (fallback)")
	force := fs.Bool("force", false, "deploy even if the router is already up to date")
	dryRun := fs.Bool("dry-run", false, "report the action without changing the router")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	opts := admin.DeployOptions{
		Address: *address,
		Port:    *port,
		Auth: deploy.Auth{
			User:     *user,
			KeyPath:  *keyPath,
			KeyPass:  *keyPass,
			UseAgent: *useAgent,
			Password: *password,
		},
	}

	switch sub {
	case "status":
		st, err := admin.Status(cfg, opts)
		if err != nil {
			return err
		}
		if !st.Installed {
			fmt.Printf("router=%s installed=false desired_hash=%s\n", st.Router, st.DesiredHash)
			return nil
		}
		fmt.Printf("router=%s installed=true up_to_date=%t installed_hash=%s desired_hash=%s\n",
			st.Router, st.UpToDate, st.InstalledHash, st.DesiredHash)
		return nil

	case "uninstall":
		addr, applied, err := admin.Uninstall(cfg, opts, *dryRun)
		if err != nil {
			return err
		}
		if !applied {
			fmt.Printf("router=%s would uninstall mkpk-tt-* layer\n", addr)
			return nil
		}
		fmt.Printf("router=%s uninstalled\n", addr)
		return nil

	default:
		res, err := admin.Apply(cfg, opts, *force, *dryRun)
		if err != nil {
			return err
		}
		switch {
		case res.Action == "skip":
			fmt.Printf("router=%s already up to date hash=%s\n", res.Router, res.Hash)
		case !res.Applied:
			fmt.Printf("router=%s would %s installed_hash=%s desired_hash=%s\n", res.Router, res.Action, res.InstalledHash, res.Hash)
		default:
			fmt.Printf("router=%s %s ok hash=%s\n", res.Router, res.Action, res.Hash)
		}
		return nil
	}
}

func serveCmd(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "mkpk.yaml", "config path")
	addr := fs.String("addr", "127.0.0.1:8765", "listen address (loopback only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	token, err := admin.GenerateSecret(24)
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: *addr, Handler: web.Handler(*configPath, token)}
	fmt.Printf("mkpk provision UI: http://%s/\n", *addr)
	fmt.Printf("config: %s (loopback only, Ctrl-C to stop)\n", *configPath)
	return srv.ListenAndServe()
}

func printSummary(path string, s admin.Summary) {
	fmt.Printf("config=%s status=valid\n", path)
	fmt.Printf("router name=%s address=%s\n", s.Router.Name, s.Router.Address)
	fmt.Printf("defaults bucket_seconds=%d stage_timeout=%s token_hit_timeout=%s allowed_timeout=%s used_timeout=%s\n",
		s.Defaults.BucketSeconds, s.Defaults.StageTimeout, s.Defaults.TokenHitTimeout, s.Defaults.AllowedTimeout, s.Defaults.UsedTimeout)
	fmt.Printf("services count=%d names=%s\n", len(s.Services), joinServiceNames(s.Services))
	for _, svc := range s.Services {
		fmt.Printf("service name=%s service_name=%s stage1=%d stage2=%d token=%d allowed_list=%s nat_enabled=%t nat_dst_port=%d nat_to=%s:%d notify_enabled=%t notify_channel=%s\n",
			svc.Name, svc.ServiceName, svc.Stage1Port, svc.Stage2Port, svc.TokenPort, svc.AllowedList,
			svc.NATEnabled, svc.NATDstPort, svc.NATToAddress, svc.NATToPort, svc.NotifyEnabled, svc.NotifyChannel)
	}
	fmt.Printf("clients count=%d names=%s\n", len(s.Clients), joinClientNames(s.Clients))
	for _, cl := range s.Clients {
		fmt.Printf("client name=%s client_id=%s service=%s psk=set\n", cl.Name, cl.ClientID, cl.Service)
	}
}

func joinServiceNames(services []admin.ServiceSummary) string {
	names := make([]string, 0, len(services))
	for _, s := range services {
		names = append(names, s.Name)
	}
	return joinNames(names)
}

func joinClientNames(clients []admin.ClientSummary) string {
	names := make([]string, 0, len(clients))
	for _, c := range clients {
		names = append(names, c.Name)
	}
	return joinNames(names)
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ",")
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
