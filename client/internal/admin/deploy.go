package admin

import (
	"fmt"

	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/deploy"
)

// DeployOptions carries the connection parameters for a deploy operation.
type DeployOptions struct {
	Address string // override; cfg.Router.Address when empty
	Port    int    // 0 → 22
	Auth    deploy.Auth
}

// StatusResult is the detected install state of a router.
type StatusResult struct {
	Router        string `json:"router"`
	Installed     bool   `json:"installed"`
	UpToDate      bool   `json:"up_to_date"`
	InstalledHash string `json:"installed_hash"`
	DesiredHash   string `json:"desired_hash"`
}

// ApplyResult reports what an install/update did or would do.
type ApplyResult struct {
	Router        string `json:"router"`
	Action        string `json:"action"` // skip | install | update
	Applied       bool   `json:"applied"`
	Hash          string `json:"hash"`
	InstalledHash string `json:"installed_hash"`
}

// Status connects and reports whether mkpk is installed and up to date.
func Status(cfg config.Config, o DeployOptions) (StatusResult, error) {
	c, addr, err := connect(cfg, o)
	if err != nil {
		return StatusResult{}, err
	}
	defer c.Close()
	state, err := c.Detect()
	if err != nil {
		return StatusResult{}, err
	}
	desired := cfg.Hash()
	return StatusResult{
		Router:        addr,
		Installed:     state.Installed,
		UpToDate:      state.Installed && state.Hash == desired,
		InstalledHash: state.Hash,
		DesiredHash:   desired,
	}, nil
}

// Apply installs or updates the mkpk layer. With dryRun it only reports the
// action; without force it skips when the router is already up to date.
func Apply(cfg config.Config, o DeployOptions, force, dryRun bool) (ApplyResult, error) {
	rendered, err := Render(cfg, "")
	if err != nil {
		return ApplyResult{}, err
	}
	c, addr, err := connect(cfg, o)
	if err != nil {
		return ApplyResult{}, err
	}
	defer c.Close()
	state, err := c.Detect()
	if err != nil {
		return ApplyResult{}, err
	}
	desired := cfg.Hash()
	res := ApplyResult{Router: addr, Hash: desired, InstalledHash: state.Hash}
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

// Uninstall removes the mkpk layer. Returns the resolved router address.
func Uninstall(cfg config.Config, o DeployOptions, dryRun bool) (string, bool, error) {
	c, addr, err := connect(cfg, o)
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

func connect(cfg config.Config, o DeployOptions) (*deploy.Client, string, error) {
	addr := o.Address
	if addr == "" {
		addr = cfg.Router.Address
	}
	if addr == "" {
		return nil, "", fmt.Errorf("router address is required (option or router.address)")
	}
	port := o.Port
	if port == 0 {
		port = 22
	}
	c, err := deploy.Connect(addr, port, o.Auth)
	if err != nil {
		return nil, addr, err
	}
	return c, addr, nil
}
