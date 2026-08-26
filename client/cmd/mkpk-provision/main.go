package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"mikrotik-psk-knock/client/internal/admin"
	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/deploy"
	"mikrotik-psk-knock/client/internal/token"
	"mikrotik-psk-knock/client/internal/version"
	"mikrotik-psk-knock/client/internal/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// printJSON writes v as indented JSON to stdout — the machine-readable output
// used by the --json flags for automation (Ansible, scripts).
func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
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
	case "router":
		return routerCmd(args[1:])
	case "client", "user":
		return clientCmd(args[1:])
	case "service":
		return serviceCmd(args[1:])
	case "token":
		return tokenCmd(args[1:])
	case "routeros":
		return routerosCmd(args[1:])
	case "export":
		return exportCmd(args[1:])
	case "deploy":
		return deployCmd(args[1:])
	case "test":
		return testCmd(args[1:])
	case "serve":
		return serveCmd(args[1:])
	case "passwd":
		return passwdCmd(args[1:])
	case "sshkey":
		return sshKeyCmd(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("mkpk-provision %s\n", version.String())
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
  mkpk-provision secret generate [--bytes 32]
  mkpk-provision config validate --config mkpk.yaml [--json]
  mkpk-provision profile init --out mkpk.yaml --router-name r1 --router-address host
  mkpk-provision router set --config mkpk.yaml --name r1 --address host [--ssh-user admin] [--ssh-key ~/.ssh/id_ed25519] [--ssh-agent] [--ssh-port 22]
  mkpk-provision router remove --config mkpk.yaml --name r1
  mkpk-provision service add --config mkpk.yaml [--router r1] --name ssh --stage1-port 41011 --stage2-port 41012 --token-port 41013 --target-type forward --target-port 2022 --target-to-address 192.0.2.10 --target-to-port 22
  mkpk-provision service add ... --target-type local --target-port 8291   # gate a port on the router itself
  mkpk-provision user add --config mkpk.yaml [--router r1] --name laptop --services ssh,web   # grants access on the router
  mkpk-provision user remove --config mkpk.yaml [--router r1] --name laptop [--all-routers]
  mkpk-provision token --config mkpk.yaml [--router r1] --client laptop [--service ssh] [--bucket N] [--debug]
  mkpk-provision routeros render --config mkpk.yaml [--router r1] [--out generated.rsc]
  mkpk-provision export --config mkpk.yaml --user laptop [--router r1] [--out laptop.mkpk]   # all routers when --router omitted
  mkpk-provision deploy [status|uninstall] --config mkpk.yaml [--router r1] [--json]   # creds come from the router
  mkpk-provision test --config mkpk.yaml [--router r1] --client laptop [--service ssh] [--wait 4s]   # end-to-end knock test over SSH
  mkpk-provision serve --config mkpk.yaml [--addr 127.0.0.1:8765] [--behind-proxy]
  mkpk-provision passwd --config mkpk.yaml [--password …]   # admin password for the networked UI
  mkpk-provision sshkey [show|create] --config mkpk.yaml [--replace]   # deploy key of this instance
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
	configPath := fs.String("config", config.DefaultPath(), "config path")
	asJSON := fs.Bool("json", false, "machine-readable JSON summary")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(admin.Summarize(cfg))
	}
	printSummary(*configPath, admin.Summarize(cfg))
	return nil
}

func profileCmd(args []string) error {
	if len(args) == 0 || args[0] != "init" {
		return fmt.Errorf("usage: mkpk-provision profile init --out mkpk.yaml --router-address host")
	}
	fs := flag.NewFlagSet("profile init", flag.ContinueOnError)
	outPath := fs.String("out", config.DefaultPath(), "output config path")
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

func routerCmd(args []string) error {
	if len(args) == 0 || (args[0] != "set" && args[0] != "remove") {
		return fmt.Errorf("usage: mkpk-provision router [set|remove] --config mkpk.yaml --name r1 [--address host] [--ssh-user admin] [--ssh-key path] [--ssh-agent] [--ssh-port 22]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("router "+sub, flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	name := fs.String("name", "", "router name")
	address := fs.String("address", "", "public router address (what clients knock)")
	allowedTimeout := fs.String("allowed-timeout", "", "router-wide default allowed-list TTL (e.g. 3m); empty keeps current")
	sshAddress := fs.String("ssh-address", "", "SSH deploy address override; router address when empty")
	sshUser := fs.String("ssh-user", "", "SSH username for deploy")
	sshKey := fs.String("ssh-key", "", "SSH private key path for deploy")
	sshKeyPass := fs.String("ssh-key-pass", "", "passphrase for the SSH key")
	sshAgent := fs.Bool("ssh-agent", false, "use ssh-agent for deploy")
	sshPassword := fs.String("ssh-password", "", "SSH password for deploy (fallback)")
	sshPort := fs.Int("ssh-port", 0, "SSH port for deploy (default 22)")
	notifyWebhook := fs.Bool("notify-webhook", false, "enable webhook channel")
	notifyURL := fs.String("notify-url", "", "webhook notification URL")
	notifyTelegram := fs.Bool("notify-telegram", false, "enable telegram channel")
	notifyTgToken := fs.String("notify-telegram-bot-token", "", "telegram bot token")
	notifyTgChat := fs.String("notify-telegram-chat-id", "", "telegram chat id")
	notifyEmail := fs.Bool("notify-email", false, "enable email channel")
	notifyEmailTo := fs.String("notify-email-to", "", "email recipient")
	notifyEmailFrom := fs.String("notify-email-from", "", "email sender")
	notifyEmailServer := fs.String("notify-email-server", "", "SMTP server host")
	notifyEmailPort := fs.Int("notify-email-port", 0, "SMTP port (default 587)")
	notifyEmailTLS := fs.String("notify-email-tls", "", "SMTP tls: no, yes or starttls (default starttls)")
	notifyEmailUser := fs.String("notify-email-user", "", "SMTP username")
	notifyEmailPassword := fs.String("notify-email-password", "", "SMTP password")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if sub == "remove" {
		cfg, err = admin.RemoveRouter(cfg, *name)
		if err != nil {
			return err
		}
		if err := admin.SaveConfig(*configPath, cfg); err != nil {
			return err
		}
		fmt.Printf("router removed config=%s name=%s\n", *configPath, *name)
		return nil
	}
	// set: keep secrets when the flag is left blank on an existing router.
	dep := config.Deploy{
		Address: *sshAddress, Port: *sshPort, User: *sshUser, KeyPath: *sshKey,
		KeyPass: *sshKeyPass, UseAgent: *sshAgent, Password: *sshPassword,
	}
	notify := config.Notify{
		Webhook:  config.NotifyWebhook{Enabled: *notifyWebhook, URL: *notifyURL},
		Telegram: config.NotifyTelegram{Enabled: *notifyTelegram, BotToken: *notifyTgToken, ChatID: *notifyTgChat},
		Email: config.NotifyEmail{
			Enabled: *notifyEmail, To: *notifyEmailTo, From: *notifyEmailFrom, Server: *notifyEmailServer,
			Port: *notifyEmailPort, TLS: *notifyEmailTLS, User: *notifyEmailUser, Password: *notifyEmailPassword,
		},
	}
	if existing, ok := cfg.Routers[*name]; ok {
		if *address == "" {
			*address = existing.Address
		}
		if dep.KeyPass == "" {
			dep.KeyPass = existing.Deploy.KeyPass
		}
		if dep.Password == "" {
			dep.Password = existing.Deploy.Password
		}
		if notify.Telegram.BotToken == "" {
			notify.Telegram.BotToken = existing.Notify.Telegram.BotToken
		}
		if notify.Email.Password == "" {
			notify.Email.Password = existing.Notify.Email.Password
		}
	}
	cfg, err = admin.SetRouter(cfg, admin.RouterOptions{Name: *name, Address: *address, Deploy: dep, Notify: notify, AllowedTimeout: *allowedTimeout})
	if err != nil {
		return err
	}
	if err := admin.SaveConfig(*configPath, cfg); err != nil {
		return err
	}
	r := cfg.Routers[*name]
	fmt.Printf("router set config=%s name=%s address=%s ssh_user=%s ssh_agent=%t notify(webhook=%t telegram=%t email=%t)\n",
		*configPath, *name, *address, r.Deploy.User, r.Deploy.UseAgent, r.Notify.Webhook.Enabled, r.Notify.Telegram.Enabled, r.Notify.Email.Enabled)
	return nil
}

func serviceCmd(args []string) error {
	if len(args) == 0 || args[0] != "add" {
		return fmt.Errorf("usage: mkpk-provision service add --config mkpk.yaml --name ssh --stage1-port 41011 --stage2-port 41012 --token-port 41013 --nat-dst-port 2022 --nat-to-address 192.0.2.10 --nat-to-port 22")
	}
	fs := flag.NewFlagSet("service add", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	router := fs.String("router", "", "router name; sole router when empty")
	name := fs.String("name", "", "service map key")
	serviceName := fs.String("service-name", "", "service_name; --name when empty")
	stage1Port := fs.Int("stage1-port", 0, "stage1 UDP port")
	stage2Port := fs.Int("stage2-port", 0, "stage2 UDP port")
	tokenPort := fs.Int("token-port", 0, "token UDP port")
	randomPorts := fs.Bool("random-ports", false, "fill unset knock ports with free random ones")
	allowedList := fs.String("allowed-list", "", "RouterOS allowed address-list; mkpk-tt-allowed-<name> when empty")
	allowedTimeout := fs.String("allowed-timeout", "", "how long a client stays allowed after a knock (e.g. 10m); inherits the router default when empty")
	targetType := fs.String("target-type", "forward", "target type: forward (dst-nat) or local (router input)")
	targetProto := fs.String("target-protocol", "tcp", "target protocol: tcp or udp")
	targetPort := fs.Int("target-port", 0, "dst-port on the router the client reaches")
	targetComment := fs.String("target-comment", "", "stable RouterOS rule comment")
	targetToAddress := fs.String("target-to-address", "", "internal service address (forward only)")
	targetToPort := fs.Int("target-to-port", 0, "internal service port (forward only)")
	force := fs.Bool("force", false, "replace existing service")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	rn, err := pickRouter(cfg, *router)
	if err != nil {
		return err
	}
	if *randomPorts {
		free, err := admin.SuggestPorts(cfg, rn, 3)
		if err != nil {
			return err
		}
		if *stage1Port == 0 {
			*stage1Port = free[0]
		}
		if *stage2Port == 0 {
			*stage2Port = free[1]
		}
		if *tokenPort == 0 {
			*tokenPort = free[2]
		}
	}
	cfg, err = admin.AddService(cfg, rn, admin.ServiceOptions{
		Name:           *name,
		ServiceName:    *serviceName,
		Stage1Port:     *stage1Port,
		Stage2Port:     *stage2Port,
		TokenPort:      *tokenPort,
		AllowedList:    *allowedList,
		AllowedTimeout: *allowedTimeout,
		Target: config.Target{
			Type:      *targetType,
			Protocol:  *targetProto,
			Port:      *targetPort,
			Comment:   *targetComment,
			ToAddress: *targetToAddress,
			ToPort:    *targetToPort,
		},
		Force: *force,
	})
	if err != nil {
		return err
	}
	if err := admin.SaveConfig(*configPath, cfg); err != nil {
		return err
	}
	svc := cfg.Routers[rn].Services[*name]
	fmt.Printf("service added config=%s router=%s name=%s service_name=%s stage1=%d stage2=%d token=%d target=%s/%s port=%d to=%s:%d\n",
		*configPath, rn, *name, svc.ServiceName, svc.Stage1Port, svc.Stage2Port, svc.TokenPort,
		svc.Target.Type, svc.Target.Protocol, svc.Target.Port, svc.Target.ToAddress, svc.Target.ToPort)
	return nil
}

func clientCmd(args []string) error {
	sub := "add"
	if len(args) > 0 && (args[0] == "add" || args[0] == "remove") {
		sub = args[0]
		args = args[1:]
	} else if len(args) > 0 {
		return fmt.Errorf("usage: mkpk-provision user [add|remove] --config mkpk.yaml --name laptop [--router r1] [--services a,b]")
	}
	fs := flag.NewFlagSet("user "+sub, flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	router := fs.String("router", "", "router name; sole router when empty")
	name := fs.String("name", "", "user map key")
	clientID := fs.String("client-id", "", "client_id; --name when empty")
	services := fs.String("services", "", "comma-separated service names on the router")
	pskFlag := fs.String("psk", "", "per-router PSK; generated when empty for a new grant")
	allRouters := fs.Bool("all-routers", false, "remove: drop the user from all routers")
	force := fs.Bool("force", false, "replace an existing grant on the router")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if sub == "remove" {
		if *name == "" {
			return fmt.Errorf("--name is required")
		}
		if *allRouters {
			cfg, err = admin.RemoveUser(cfg, *name)
		} else {
			rn, e := pickRouter(cfg, *router)
			if e != nil {
				return e
			}
			cfg, err = admin.RemoveUserAccess(cfg, rn, *name)
		}
		if err != nil {
			return err
		}
		if err := admin.SaveConfig(*configPath, cfg); err != nil {
			return err
		}
		fmt.Printf("user removed config=%s name=%s all_routers=%t\n", *configPath, *name, *allRouters)
		return nil
	}
	rn, err := pickRouter(cfg, *router)
	if err != nil {
		return err
	}
	res, err := admin.AddUser(cfg, rn, admin.UserOptions{
		Name:     *name,
		ClientID: *clientID,
		Services: splitList(*services),
		PSK:      *pskFlag,
		Force:    *force,
	})
	if err != nil {
		return err
	}
	if err := admin.SaveConfig(*configPath, res.Config); err != nil {
		return err
	}
	access := res.Config.Users[*name].Access[rn]
	fmt.Printf("user granted config=%s router=%s name=%s client_id=%s services=%s psk=%s\n",
		*configPath, rn, *name, res.Config.Users[*name].ClientID, strings.Join(access.Services, ","), res.PSKSource)
	return nil
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func pickRouter(cfg config.Config, name string) (string, error) {
	if name != "" {
		if _, ok := cfg.Routers[name]; !ok {
			return "", fmt.Errorf("unknown router %q", name)
		}
		return name, nil
	}
	if len(cfg.Routers) != 1 {
		return "", fmt.Errorf("--router is required (config has %d routers)", len(cfg.Routers))
	}
	for n := range cfg.Routers {
		return n, nil
	}
	return "", fmt.Errorf("config has no routers")
}

func tokenCmd(args []string) error {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	routerName := fs.String("router", "", "router name; sole router when empty")
	clientName := fs.String("client", "", "user name")
	serviceName := fs.String("service", "", "service name; sole service when empty")
	bucketFlag := fs.String("bucket", "", "override time bucket")
	debug := fs.Bool("debug", false, "print token metadata")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := loadResolved(*configPath, *routerName, *clientName, *serviceName)
	if err != nil {
		return err
	}
	window := token.InspectWindow(time.Now(), res.Router.Defaults.BucketSeconds)
	bucket := window.Bucket
	if *bucketFlag != "" {
		bucket, err = strconv.ParseInt(*bucketFlag, 10, 64)
		if err != nil {
			return fmt.Errorf("--bucket: %w", err)
		}
	}
	value := token.Compute(res.PSK, res.Service.ServiceName, res.ClientID, bucket)
	if *debug {
		fmt.Printf("service=%s client_id=%s bucket=%d token=%s\n", res.Service.ServiceName, res.ClientID, bucket, value)
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
	configPath := fs.String("config", config.DefaultPath(), "config path")
	router := fs.String("router", "", "router name; sole router when empty")
	outPath := fs.String("out", "", "output .rsc path; stdout when empty")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	rn, err := pickRouter(cfg, *router)
	if err != nil {
		return err
	}
	rendered, err := admin.Render(cfg, rn)
	if err != nil {
		return err
	}
	if *outPath == "" {
		fmt.Print(rendered)
		return nil
	}
	return os.WriteFile(*outPath, []byte(rendered), 0600)
}

func exportCmd(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	router := fs.String("router", "", "single router to export; all the user's routers when empty")
	user := fs.String("user", "", "user name")
	outPath := fs.String("out", "", "output file; stdout when empty")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *user == "" {
		return fmt.Errorf("--user is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	blob, err := admin.ExportUser(cfg, *user, *router)
	if err != nil {
		return err
	}
	if *outPath == "" {
		fmt.Println(blob)
		return nil
	}
	if err := os.WriteFile(*outPath, []byte(blob+"\n"), 0600); err != nil {
		return err
	}
	scope := *router
	if scope == "" {
		scope = "all-routers"
	}
	fmt.Printf("invite for user=%s scope=%s written to %s\n", *user, scope, *outPath)
	return nil
}

func deployCmd(args []string) error {
	sub := "install"
	if len(args) > 0 && (args[0] == "status" || args[0] == "uninstall") {
		sub = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	routerName := fs.String("router", "", "router name; sole router when empty")
	// Connection credentials live on the router (see `router set`). These flags
	// are optional per-call overrides; empty means "use the router's".
	address := fs.String("address", "", "override router address")
	user := fs.String("user", "", "override SSH username")
	port := fs.Int("port", 0, "override SSH port")
	keyPath := fs.String("key", "", "override SSH private key path")
	keyPass := fs.String("key-pass", "", "override SSH key passphrase")
	useAgent := fs.Bool("agent", false, "also try ssh-agent")
	password := fs.String("password", "", "override SSH password (fallback)")
	force := fs.Bool("force", false, "deploy even if the router is already up to date")
	dryRun := fs.Bool("dry-run", false, "report the action without changing the router")
	asJSON := fs.Bool("json", false, "machine-readable JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	rn, err := pickRouter(cfg, *routerName)
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
		st, err := admin.Status(cfg, rn, opts)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(st)
		}
		if !st.Installed {
			fmt.Printf("router=%s installed=false desired_hash=%s\n", st.Router, st.DesiredHash)
			return nil
		}
		fmt.Printf("router=%s installed=true up_to_date=%t installed_hash=%s desired_hash=%s\n",
			st.Router, st.UpToDate, st.InstalledHash, st.DesiredHash)
		return nil

	case "uninstall":
		res, err := admin.Uninstall(cfg, rn, opts, *dryRun)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(res)
		}
		if !res.Applied {
			fmt.Printf("router=%s would uninstall mkpk-tt-* layer\n", res.Router)
			return nil
		}
		fmt.Printf("router=%s uninstalled\n", res.Router)
		return nil

	default:
		res, err := admin.Apply(cfg, rn, opts, *force, *dryRun)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(res)
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

func testCmd(args []string) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	routerName := fs.String("router", "", "router name; sole router when empty")
	clientName := fs.String("client", "", "user name to knock as")
	serviceName := fs.String("service", "", "service name; sole service when empty")
	wait := fs.Duration("wait", 4*time.Second, "wait after knocking before reading router state")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *clientName == "" {
		return fmt.Errorf("--client is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	rn, err := pickRouter(cfg, *routerName)
	if err != nil {
		return err
	}
	logf := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	res, err := admin.KnockTest(cfg, rn, *serviceName, *clientName, *wait,
		admin.DeployOptions{OnLog: func(line string) { fmt.Println("  " + line) }}, logf)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	fmt.Printf("\n%s: %s\n", map[bool]string{true: "PASS", false: "FAIL"}[res.Pass], res.Diagnosis)
	return nil
}

func serveCmd(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	addr := fs.String("addr", "127.0.0.1:8765", "listen address; non-loopback needs a password + --behind-proxy")
	behindProxy := fs.Bool("behind-proxy", envBool("MKPK_BEHIND_PROXY"),
		"a reverse proxy terminates TLS in front of this instance")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Shared-instance mode is on as soon as an admin password exists (or the
	// environment supplies the first one). Local single-operator use is
	// unchanged: loopback + a per-process token, no password.
	credPath := web.CredentialsPath(*configPath)
	// Whether a password already existed decides the message below, so check
	// before bootstrapping — afterwards the file exists either way.
	_, hadCredentials, err := web.LoadCredentials(credPath)
	if err != nil {
		return err
	}
	creds, hasPassword, err := web.BootstrapPassword(credPath, os.Getenv("MKPK_ADMIN_PASSWORD"))
	if err != nil {
		return err
	}
	switch {
	case os.Getenv("MKPK_ADMIN_PASSWORD") != "" && hadCredentials:
		fmt.Fprintf(os.Stderr, "note: %s already exists — MKPK_ADMIN_PASSWORD ignored (use `mkpk-provision passwd` to change it)\n", credPath)
	case os.Getenv("MKPK_ADMIN_PASSWORD") != "" && hasPassword:
		fmt.Printf("admin password set from MKPK_ADMIN_PASSWORD: %s\n", credPath)
	}

	// The config holds SSH credentials for every router, notification tokens
	// and every user's PSK. Reaching it over the network without a password is
	// never what someone means, so refuse instead of trusting a typo.
	if !isLoopbackAddr(*addr) {
		if !hasPassword {
			return fmt.Errorf("refusing to listen on %s without an admin password: set MKPK_ADMIN_PASSWORD or run `mkpk-provision passwd`", *addr)
		}
		if !*behindProxy {
			return fmt.Errorf("refusing to serve plain HTTP on %s: terminate TLS in a reverse proxy and pass --behind-proxy (or MKPK_BEHIND_PROXY=1)", *addr)
		}
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)
	var handler http.Handler
	if hasPassword {
		handler = web.LogRequests(web.AuthHandler(*configPath, web.NewAuth(credPath, creds, *behindProxy)), logger)
	} else {
		token, err := admin.GenerateSecret(24)
		if err != nil {
			return err
		}
		handler = web.LogRequests(web.Handler(*configPath, token), logger)
	}
	srv := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	fmt.Printf("mkpk provision UI: http://%s/\n", *addr)
	mode := "loopback only, no password"
	if hasPassword {
		mode = "password required"
		if *behindProxy {
			mode += ", behind a TLS proxy"
		}
	}
	fmt.Printf("config: %s (%s, Ctrl-C to stop)\n", *configPath, mode)
	return srv.ListenAndServe()
}

// isLoopbackAddr reports whether a listen address stays on the local machine.
// An empty host (":8765") means every interface — not loopback.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	switch host {
	case "localhost":
		return true
	case "":
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// sshKeyCmd shows or creates the instance deploy key — the identity this
// installation uses to reach routers, as opposed to an admin's personal key.
func sshKeyCmd(args []string) error {
	sub := "show"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("sshkey", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	replace := fs.Bool("replace", false, "replace an existing key (invalidates it on every router)")
	comment := fs.String("comment", "mkpk-provision", "key comment")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch sub {
	case "show":
		info, err := admin.ReadInstanceKey(*configPath)
		if err != nil {
			return err
		}
		if !info.Exists {
			fmt.Printf("no instance key yet — create one: mkpk-provision sshkey create --config %s\n", *configPath)
			return nil
		}
		fmt.Printf("path=%s\nfingerprint=%s\n%s\n", info.Path, info.Fingerprint, info.PublicKey)
		return nil
	case "create":
		info, err := admin.CreateInstanceKey(*configPath, *comment, *replace)
		if err != nil {
			return err
		}
		fmt.Printf("created %s\nfingerprint=%s\n%s\n", info.Path, info.Fingerprint, info.PublicKey)
		fmt.Printf("import it on each router, then set the router's key_path to %s\n", info.Path)
		return nil
	default:
		return fmt.Errorf("usage: mkpk-provision sshkey [show|create]")
	}
}

// passwdCmd sets or replaces the admin password used by the networked UI.
func passwdCmd(args []string) error {
	fs := flag.NewFlagSet("passwd", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "config path")
	password := fs.String("password", "", "new password; read from stdin when empty")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pw := *password
	if pw == "" {
		fmt.Fprint(os.Stderr, "new admin password: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("reading password: %w", err)
		}
		pw = strings.TrimRight(line, "\r\n")
	}
	rec, err := web.HashPassword(pw)
	if err != nil {
		return err
	}
	path := web.CredentialsPath(*configPath)
	if err := web.SaveCredentials(path, rec); err != nil {
		return err
	}
	fmt.Printf("admin password updated: %s\n", path)
	fmt.Println("all existing sessions on a running instance stay valid until it restarts")
	return nil
}

func printSummary(path string, s admin.Summary) {
	fmt.Printf("config=%s status=valid routers=%d\n", path, len(s.Routers))
	for _, r := range s.Routers {
		fmt.Printf("router name=%s address=%s hash=%s\n", r.Name, r.Address, r.Hash[:min(16, len(r.Hash))])
		sshAddr := r.Deploy.Address
		if sshAddr == "" {
			sshAddr = r.Address + " (=address)"
		}
		fmt.Printf("  deploy configured=%t ssh_address=%s ssh_user=%s ssh_key=%s ssh_agent=%t ssh_port=%d password_set=%t\n",
			r.Deploy.Configured, sshAddr, r.Deploy.User, r.Deploy.KeyPath, r.Deploy.UseAgent, r.Deploy.Port, r.Deploy.PasswordSet)
		fmt.Printf("  notify active=%t webhook=%t telegram=%t email=%t\n", r.Notify.Active, r.Notify.WebhookEnabled, r.Notify.TelegramEnabled, r.Notify.EmailEnabled)
		fmt.Printf("  defaults bucket_seconds=%d stage_timeout=%s token_hit_timeout=%s allowed_timeout=%s used_timeout=%s\n",
			r.Defaults.BucketSeconds, r.Defaults.StageTimeout, r.Defaults.TokenHitTimeout, r.Defaults.AllowedTimeout, r.Defaults.UsedTimeout)
		for _, svc := range r.Services {
			state := "enabled"
			if !svc.Enabled {
				state = "disabled"
			}
			fmt.Printf("  service name=%s [%s] service_name=%s stage1=%d stage2=%d token=%d allowed_list=%s target=%s/%s port=%d to=%s:%d\n",
				svc.Name, state, svc.ServiceName, svc.Stage1Port, svc.Stage2Port, svc.TokenPort, svc.AllowedList,
				svc.TargetType, svc.TargetProtocol, svc.TargetPort, svc.TargetToAddress, svc.TargetToPort)
		}
		for _, cl := range r.Clients {
			fmt.Printf("  access user=%s client_id=%s services=%s psk=set\n", cl.Name, cl.ClientID, strings.Join(cl.Services, ","))
		}
	}
	fmt.Printf("users=%d\n", len(s.Users))
	for _, u := range s.Users {
		fmt.Printf("user name=%s client_id=%s\n", u.Name, u.ClientID)
		for _, a := range u.Access {
			fmt.Printf("  access router=%s services=%s psk=set\n", a.Router, strings.Join(a.Services, ","))
		}
	}
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
	rn, err := pickRouter(cfg, routerName)
	if err != nil {
		return config.Resolved{}, err
	}
	return cfg.Resolve(clientName, rn, serviceName)
}
