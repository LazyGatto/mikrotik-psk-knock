package admin

import (
	"fmt"

	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/deploy"
)

// DeployOptions carries optional per-call overrides for a deploy operation. The
// connection parameters normally live on the router (config.Deploy); an empty
// DeployOptions uses them as-is. Non-empty fields here take precedence, letting
// CLI power users override without editing the config.
type DeployOptions struct {
	Address string // override; router.Address when empty
	Port    int    // override; router deploy port (then 22) when 0
	Auth    deploy.Auth
}

// StatusResult is the detected install state of a router.
type StatusResult struct {
	Router        string `json:"router"`
	Address       string `json:"address"`
	Installed     bool   `json:"installed"`
	UpToDate      bool   `json:"up_to_date"`
	InstalledHash string `json:"installed_hash"`
	DesiredHash   string `json:"desired_hash"`
}

// ApplyResult reports what an install/update did or would do.
type ApplyResult struct {
	Router        string `json:"router"`
	Address       string `json:"address"`
	Action        string `json:"action"` // skip | install | update
	Applied       bool   `json:"applied"`
	Hash          string `json:"hash"`
	InstalledHash string `json:"installed_hash"`
}

// Status connects to routerName and reports whether mkpk is installed and up to date.
func Status(cfg config.Config, routerName string, o DeployOptions) (StatusResult, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return StatusResult{}, err
	}
	c, addr, err := connect(r, o)
	if err != nil {
		return StatusResult{}, err
	}
	defer c.Close()
	state, err := c.Detect()
	if err != nil {
		return StatusResult{}, err
	}
	desired := cfg.RouterHash(routerName)
	return StatusResult{
		Router:        routerName,
		Address:       addr,
		Installed:     state.Installed,
		UpToDate:      state.Installed && state.Hash == desired,
		InstalledHash: state.Hash,
		DesiredHash:   desired,
	}, nil
}

// Apply installs or updates the mkpk layer on routerName. With dryRun it only
// reports the action; without force it skips when already up to date.
func Apply(cfg config.Config, routerName string, o DeployOptions, force, dryRun bool) (ApplyResult, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return ApplyResult{}, err
	}
	rendered, err := Render(cfg, routerName)
	if err != nil {
		return ApplyResult{}, err
	}
	c, addr, err := connect(r, o)
	if err != nil {
		return ApplyResult{}, err
	}
	defer c.Close()
	state, err := c.Detect()
	if err != nil {
		return ApplyResult{}, err
	}
	desired := cfg.RouterHash(routerName)
	res := ApplyResult{Router: routerName, Address: addr, Hash: desired, InstalledHash: state.Hash}
	if state.Installed && state.Hash == desired && !force {
		res.Action = "skip"
		return res, nil
	}
	res.Action = "install"
	if state.Installed {
		res.Action = "update"
	}
	if dryRun {
		return res, nil
	}
	if err := c.Deploy([]byte(rendered)); err != nil {
		return res, err
	}
	res.Applied = true
	return res, nil
}

// Uninstall removes the mkpk layer from routerName.
func Uninstall(cfg config.Config, routerName string, o DeployOptions, dryRun bool) (string, bool, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return "", false, err
	}
	c, addr, err := connect(r, o)
	if err != nil {
		return "", false, err
	}
	defer c.Close()
	if dryRun {
		return addr, false, nil
	}
	if err := c.Uninstall(); err != nil {
		return addr, false, err
	}
	return addr, true, nil
}

func connect(r config.Router, o DeployOptions) (*deploy.Client, string, error) {
	addr := o.Address
	if addr == "" {
		addr = r.Address
	}
	if addr == "" {
		return nil, "", fmt.Errorf("router address is required (option or router address)")
	}
	port := o.Port
	if port == 0 {
		port = r.Deploy.Port
	}
	if port == 0 {
		port = 22
	}
	c, err := deploy.Connect(addr, port, mergeAuth(r.Deploy, o.Auth))
	if err != nil {
		return nil, addr, err
	}
	return c, addr, nil
}

// mergeAuth starts from the router's stored deploy credentials and lets any
// non-empty override field win.
func mergeAuth(d config.Deploy, o deploy.Auth) deploy.Auth {
	a := deploy.Auth{
		User:     d.User,
		KeyPath:  d.KeyPath,
		KeyPass:  d.KeyPass,
		UseAgent: d.UseAgent,
		Password: d.Password,
	}
	if o.User != "" {
		a.User = o.User
	}
	if o.KeyPath != "" {
		a.KeyPath = o.KeyPath
	}
	if o.KeyPass != "" {
		a.KeyPass = o.KeyPass
	}
	if o.Password != "" {
		a.Password = o.Password
	}
	if o.UseAgent {
		a.UseAgent = true
	}
	return a
}
