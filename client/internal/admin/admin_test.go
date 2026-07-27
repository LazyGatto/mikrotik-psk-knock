package admin

import (
	"testing"

	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/invite"
	"mikrotik-psk-knock/client/internal/token"
)

const rn = "r1"

func initCfg(t *testing.T) config.Config {
	t.Helper()
	cfg, err := InitConfig(InitOptions{RouterName: rn, RouterAddress: "r.example", ServiceName: "svc", ClientName: "cli"})
	if err != nil {
		t.Fatalf("InitConfig() error = %v", err)
	}
	return cfg
}

func TestInitConfigValidAndRequiresAddress(t *testing.T) {
	cfg := initCfg(t)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("InitConfig produced invalid config: %v", err)
	}
	if cfg.Users["cli"].Access[rn].PSK == "" {
		t.Fatal("InitConfig did not generate a PSK")
	}
	if cfg.Users["cli"].Access[rn].Services[0] != "svc" {
		t.Fatal("InitConfig user not granted the service")
	}
	if cfg.Users["cli"].ClientID != "cli" {
		t.Fatal("InitConfig user client_id wrong")
	}
	if _, err := InitConfig(InitOptions{RouterName: rn, ServiceName: "svc", ClientName: "cli"}); err == nil {
		t.Fatal("InitConfig without address should error")
	}
}

func TestSetRouterCreatesUpdatesAndKeepsChildren(t *testing.T) {
	cfg := initCfg(t)

	// Create a new router with deploy credentials.
	cfg, err := SetRouter(cfg, RouterOptions{
		Name: "r2", Address: "10.0.0.2",
		Deploy: config.Deploy{User: "admin", KeyPath: "~/.ssh/id_ed25519", Port: 2222},
	})
	if err != nil {
		t.Fatalf("SetRouter(create) error = %v", err)
	}
	if got := cfg.Routers["r2"].Deploy.KeyPath; got != "~/.ssh/id_ed25519" {
		t.Fatalf("deploy key_path = %q, want the provided path", got)
	}

	// Add a service, then edit the router: address/creds change, service stays.
	cfg, err = AddService(cfg, "r2", ServiceOptions{
		Name: "svc2", Stage1Port: 51001, Stage2Port: 51002, TokenPort: 51003,
		Target: config.Target{Type: config.TargetForward, Protocol: "tcp", Port: 2223, ToAddress: "192.0.2.30", ToPort: 22},
	})
	if err != nil {
		t.Fatalf("AddService() error = %v", err)
	}
	cfg, err = SetRouter(cfg, RouterOptions{Name: "r2", Address: "10.0.0.9", Deploy: config.Deploy{UseAgent: true}})
	if err != nil {
		t.Fatalf("SetRouter(update) error = %v", err)
	}
	r2 := cfg.Routers["r2"]
	if r2.Address != "10.0.0.9" || !r2.Deploy.UseAgent || r2.Deploy.KeyPath != "" {
		t.Fatalf("update did not replace address/creds: %+v", r2.Deploy)
	}
	if _, ok := r2.Services["svc2"]; !ok {
		t.Fatal("SetRouter(update) dropped the router's services")
	}

	// A bad deploy port is rejected.
	if _, err := SetRouter(cfg, RouterOptions{Name: "r3", Address: "h", Deploy: config.Deploy{Port: 70000}}); err == nil {
		t.Fatal("SetRouter with out-of-range port should error")
	}
}

func TestSetRouterNotifyDefaultsAndSecretFreeSummary(t *testing.T) {
	cfg := initCfg(t)
	cfg, err := SetRouter(cfg, RouterOptions{
		Name: rn, Address: "r.example",
		Notify: config.Notify{Enabled: true, Channel: "telegram", Telegram: config.NotifyTelegram{BotToken: "123:secret-token", ChatID: "-100"}},
	})
	if err != nil {
		t.Fatalf("SetRouter() error = %v", err)
	}
	n := Summarize(cfg).Routers[0].Notify
	if !n.Enabled || n.Channel != "telegram" || n.TelegramChat != "-100" || !n.BotTokenSet {
		t.Fatalf("notify summary wrong: %+v", n)
	}
	// The secret token must not appear anywhere in the summary struct fields.
	if n.URL == "123:secret-token" || n.EmailUser == "123:secret-token" {
		t.Fatal("notify summary leaked the bot token")
	}
}

func TestSummarizeReportsDeployWithoutSecrets(t *testing.T) {
	cfg := initCfg(t)
	cfg, err := SetRouter(cfg, RouterOptions{
		Name: rn, Address: "r.example",
		Deploy: config.Deploy{User: "admin", KeyPath: "k", Password: "secret", KeyPass: "kp"},
	})
	if err != nil {
		t.Fatalf("SetRouter() error = %v", err)
	}
	d := Summarize(cfg).Routers[0].Deploy
	if !d.Configured || d.User != "admin" || !d.PasswordSet || !d.KeyPassSet {
		t.Fatalf("deploy summary wrong: %+v", d)
	}
	if d.User == "secret" || d.KeyPath == "secret" {
		t.Fatal("deploy summary leaked a secret")
	}
}

func TestSuggestPortsAvoidsUsedAndCollisions(t *testing.T) {
	cfg := initCfg(t) // demo service uses 41001/41002/41003 + target 2222
	ports, err := SuggestPorts(cfg, rn, 3)
	if err != nil {
		t.Fatalf("SuggestPorts() error = %v", err)
	}
	if len(ports) != 3 {
		t.Fatalf("want 3 ports, got %d", len(ports))
	}
	used := cfg.Routers[rn].UsedPorts()
	seen := map[int]bool{}
	for _, p := range ports {
		if used[p] {
			t.Fatalf("suggested port %d is already used", p)
		}
		if seen[p] {
			t.Fatalf("suggested duplicate port %d", p)
		}
		if p < 40000 || p >= 60000 {
			t.Fatalf("suggested port %d out of range", p)
		}
		seen[p] = true
	}

	// A service built from the suggestion must save without a collision.
	cfg2, err := AddService(cfg, rn, ServiceOptions{
		Name: "extra", Stage1Port: ports[0], Stage2Port: ports[1], TokenPort: ports[2],
		Target: config.Target{Type: config.TargetLocal, Protocol: "tcp", Port: 8291},
	})
	if err != nil {
		t.Fatalf("AddService() error = %v", err)
	}
	if err := cfg2.Validate(); err != nil {
		t.Fatalf("config with suggested ports should validate: %v", err)
	}
}

func TestAddServiceDefaultsAndValidation(t *testing.T) {
	cfg := initCfg(t)

	cfg, err := AddService(cfg, rn, ServiceOptions{
		Name: "svc2", Stage1Port: 43001, Stage2Port: 43002, TokenPort: 43003,
		Target: config.Target{Type: config.TargetForward, Protocol: "tcp", Port: 2022, ToAddress: "192.0.2.10", ToPort: 22},
	})
	if err != nil {
		t.Fatalf("AddService() error = %v", err)
	}
	svc := cfg.Routers[rn].Services["svc2"]
	if svc.AllowedList != "mkpk-tt-allowed-svc2" {
		t.Fatalf("allowed_list default = %q", svc.AllowedList)
	}
	if svc.Target.Comment != "mkpk-tt target svc2" {
		t.Fatalf("target comment default = %q", svc.Target.Comment)
	}
	if svc.Target.Protocol != "tcp" {
		t.Fatalf("target protocol default = %q", svc.Target.Protocol)
	}

	if _, err := AddService(cfg, rn, ServiceOptions{Name: "bad"}); err == nil {
		t.Fatal("AddService without ports should error")
	}
	if _, err := AddService(cfg, rn, ServiceOptions{Name: "svc2", Stage1Port: 1, Stage2Port: 2, TokenPort: 3, Target: config.Target{Type: config.TargetForward, Protocol: "tcp", Port: 1, ToPort: 1, ToAddress: "x"}}); err == nil {
		t.Fatal("AddService on existing name without force should error")
	}
	if _, err := AddService(cfg, "nope", ServiceOptions{Name: "x", Stage1Port: 1, Stage2Port: 2, TokenPort: 3, Target: config.Target{Type: config.TargetForward, Protocol: "tcp", Port: 1, ToPort: 1, ToAddress: "x"}}); err == nil {
		t.Fatal("AddService on unknown router should error")
	}
}

func TestSetServiceEnabled(t *testing.T) {
	cfg := initCfg(t)
	cfg, err := SetServiceEnabled(cfg, rn, "svc", false)
	if err != nil {
		t.Fatalf("SetServiceEnabled() error = %v", err)
	}
	if cfg.Routers[rn].Services["svc"].Enabled() {
		t.Fatal("service should be disabled")
	}
	cfg, _ = SetServiceEnabled(cfg, rn, "svc", true)
	if !cfg.Routers[rn].Services["svc"].Enabled() {
		t.Fatal("service should be enabled")
	}
}

func TestRemoveServiceRefusesReferenced(t *testing.T) {
	cfg := initCfg(t)
	if _, err := RemoveService(cfg, rn, "svc"); err == nil {
		t.Fatal("RemoveService should refuse a referenced service")
	}
	cfg, _ = RemoveUserAccess(cfg, rn, "cli")
	if _, err := RemoveService(cfg, rn, "svc"); err != nil {
		t.Fatalf("RemoveService after unref error = %v", err)
	}
}

func TestAddUserGeneratesPSKAndGrants(t *testing.T) {
	cfg := initCfg(t)
	cfg, _ = AddService(cfg, rn, ServiceOptions{Name: "web", Stage1Port: 42001, Stage2Port: 42002, TokenPort: 42003, Target: config.Target{Type: config.TargetForward, Protocol: "tcp", Port: 3443, ToAddress: "192.0.2.20", ToPort: 443}})

	res, err := AddUser(cfg, rn, UserOptions{Name: "phone", Services: []string{"svc", "web"}})
	if err != nil {
		t.Fatalf("AddUser() error = %v", err)
	}
	if res.PSKSource != "generated" || res.Config.Users["phone"].Access[rn].PSK == "" {
		t.Fatalf("AddUser did not generate PSK: source=%s", res.PSKSource)
	}
	if len(res.Config.Users["phone"].Access[rn].Services) != 2 {
		t.Fatal("AddUser did not grant both services")
	}

	res, err = AddUser(res.Config, rn, UserOptions{Name: "laptop", Services: []string{"svc"}, PSK: "provided-psk"})
	if err != nil {
		t.Fatalf("AddUser() error = %v", err)
	}
	if res.PSKSource != "provided" {
		t.Fatalf("AddUser psk source = %s, want provided", res.PSKSource)
	}

	if _, err := AddUser(cfg, rn, UserOptions{Name: "x", Services: []string{"missing"}}); err == nil {
		t.Fatal("AddUser with unknown service should error")
	}
}

func TestAddUserSpansRoutersAndKeepsPSK(t *testing.T) {
	cfg := initCfg(t) // router rn with service svc, user cli granted on rn
	cfg, err := SetRouter(cfg, RouterOptions{Name: "r2", Address: "10.0.0.2"})
	if err != nil {
		t.Fatalf("SetRouter() error = %v", err)
	}
	cfg, _ = AddService(cfg, "r2", ServiceOptions{Name: "s2", Stage1Port: 51001, Stage2Port: 51002, TokenPort: 51003, Target: config.Target{Type: config.TargetLocal, Protocol: "tcp", Port: 8291}})

	// Grant the same user access on r2 — access on rn must be preserved.
	res, err := AddUser(cfg, "r2", UserOptions{Name: "cli", Services: []string{"s2"}, PSK: "r2-psk"})
	if err != nil {
		t.Fatalf("AddUser(r2) error = %v", err)
	}
	u := res.Config.Users["cli"]
	if len(u.Access) != 2 {
		t.Fatalf("user should have access on 2 routers, got %d", len(u.Access))
	}
	if u.Access[rn].PSK == u.Access["r2"].PSK {
		t.Fatal("per-router PSKs must differ")
	}

	// Editing r2 services with a blank PSK must keep the r2 PSK.
	res2, err := AddUser(res.Config, "r2", UserOptions{Name: "cli", Services: []string{"s2"}, Force: true})
	if err != nil {
		t.Fatalf("AddUser(edit) error = %v", err)
	}
	if res2.PSKSource != "kept" || res2.Config.Users["cli"].Access["r2"].PSK != "r2-psk" {
		t.Fatalf("edit did not keep the r2 PSK: source=%s", res2.PSKSource)
	}
}

func TestUserEntityLifecycle(t *testing.T) {
	cfg := initCfg(t) // user "cli" with access on rn

	// Create an empty user.
	cfg, err := CreateUser(cfg, "phone", "")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if u, ok := cfg.Users["phone"]; !ok || u.ClientID != "phone" || len(u.Access) != 0 {
		t.Fatalf("empty user wrong: %+v", cfg.Users["phone"])
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config with an access-less user should validate: %v", err)
	}
	if _, err := CreateUser(cfg, "phone", ""); err == nil {
		t.Fatal("CreateUser on existing name should error")
	}

	// Rename: key and client_id both move.
	cfg, err = RenameUser(cfg, "phone", "mobile")
	if err != nil {
		t.Fatalf("RenameUser() error = %v", err)
	}
	if _, ok := cfg.Users["phone"]; ok {
		t.Fatal("old user key should be gone after rename")
	}
	if cfg.Users["mobile"].ClientID != "mobile" {
		t.Fatalf("rename did not update client_id: %+v", cfg.Users["mobile"])
	}

	// Revoking last access keeps the user entity.
	cfg, err = RemoveUserAccess(cfg, rn, "cli")
	if err != nil {
		t.Fatalf("RemoveUserAccess() error = %v", err)
	}
	if u, ok := cfg.Users["cli"]; !ok || len(u.Access) != 0 {
		t.Fatalf("user should remain with empty access after last revoke: %+v", cfg.Users["cli"])
	}

	// Whole-user delete.
	cfg, err = RemoveUser(cfg, "cli")
	if err != nil {
		t.Fatalf("RemoveUser() error = %v", err)
	}
	if _, ok := cfg.Users["cli"]; ok {
		t.Fatal("user should be gone after RemoveUser")
	}
}

func TestRotateUserPSKChangesOnlyThatPair(t *testing.T) {
	cfg := initCfg(t)
	cfg, err := SetRouter(cfg, RouterOptions{Name: "r2", Address: "10.0.0.2"})
	if err != nil {
		t.Fatalf("SetRouter() error = %v", err)
	}
	cfg, _ = AddService(cfg, "r2", ServiceOptions{Name: "s2", Stage1Port: 51001, Stage2Port: 51002, TokenPort: 51003, Target: config.Target{Type: config.TargetLocal, Protocol: "tcp", Port: 8291}})
	res, _ := AddUser(cfg, "r2", UserOptions{Name: "cli", Services: []string{"s2"}, PSK: "r2-psk", Force: true})
	cfg = res.Config

	before := cfg.Users["cli"].Access[rn].PSK
	other := cfg.Users["cli"].Access["r2"].PSK

	cfg, err = RotateUserPSK(cfg, "cli", rn)
	if err != nil {
		t.Fatalf("RotateUserPSK() error = %v", err)
	}
	if cfg.Users["cli"].Access[rn].PSK == before {
		t.Fatal("rotate did not change the PSK")
	}
	if cfg.Users["cli"].Access["r2"].PSK != other {
		t.Fatal("rotate changed the other router's PSK")
	}
	if _, err := RotateUserPSK(cfg, "cli", "ghost"); err == nil {
		t.Fatal("rotate on a router the user can't reach should error")
	}
}

func TestExportUserBlobTokensMatchConfig(t *testing.T) {
	cfg := initCfg(t)
	cfg, _ = AddService(cfg, rn, ServiceOptions{Name: "web", Stage1Port: 42001, Stage2Port: 42002, TokenPort: 42003, Target: config.Target{Type: config.TargetForward, Protocol: "tcp", Port: 3443, ToAddress: "192.0.2.20", ToPort: 443}})
	res, _ := AddUser(cfg, rn, UserOptions{Name: "phone", Services: []string{"svc", "web"}, PSK: "phone-psk-value", Force: true})
	cfg = res.Config

	blobStr, err := ExportUser(cfg, "phone", "")
	if err != nil {
		t.Fatalf("ExportUser() error = %v", err)
	}
	b, err := invite.Decode(blobStr)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(b.Routers) != 1 || len(b.Routers[0].Services) != 2 || b.Routers[0].PSK != "phone-psk-value" {
		t.Fatalf("blob wrong: %+v", b)
	}

	// The token computed from the blob must equal the token computed from the full
	// config for the same (service, user, bucket) — otherwise the client can't open.
	const bucket = 59504932
	blobCfg := b.ToConfig()
	for _, svc := range []string{"svc", "web"} {
		full, err := cfg.Resolve("phone", rn, svc)
		if err != nil {
			t.Fatalf("config Resolve %s error = %v", svc, err)
		}
		fromBlob, err := blobCfg.Resolve("phone", "r.example", svc)
		if err != nil {
			t.Fatalf("blob Resolve %s error = %v", svc, err)
		}
		ct := token.Compute(full.PSK, full.Service.ServiceName, full.ClientID, bucket)
		bt := token.Compute(fromBlob.PSK, fromBlob.Service.ServiceName, fromBlob.ClientID, bucket)
		if ct != bt {
			t.Fatalf("service %s: blob token != config token", svc)
		}
	}
}

func TestSummarizeIsSecretFreeAndOrdered(t *testing.T) {
	cfg := initCfg(t)
	s := Summarize(cfg)
	if len(s.Routers) != 1 {
		t.Fatalf("summary routers = %d", len(s.Routers))
	}
	r := s.Routers[0]
	if r.Name != rn || len(r.Services) != 1 || len(r.Clients) != 1 {
		t.Fatalf("summary shape wrong: %+v", r)
	}
	if !r.Clients[0].PSKSet {
		t.Fatal("summary should mark psk set")
	}
	if !r.Services[0].Enabled {
		t.Fatal("summary service should be enabled")
	}
	if len(s.Users) != 1 || s.Users[0].Name != "cli" || len(s.Users[0].Access) != 1 || !s.Users[0].Access[0].PSKSet {
		t.Fatalf("top-level users summary wrong: %+v", s.Users)
	}
}
