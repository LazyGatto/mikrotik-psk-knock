package admin

import (
	"fmt"

	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/deploy"
)

// Onboarding a router means installing mkpk's own service account on it, using
// the administrator's credentials exactly once. Everything afterwards — deploy,
// status, uninstall — runs as that account, and handing the router back is one
// self-removal rather than a hunt for stray keys.

// ServiceUserResult reports what onboarding did.
type ServiceUserResult struct {
	Router      string `json:"router"`
	Address     string `json:"address"`
	User        string `json:"user"`
	Policies    string `json:"policies"`
	Fingerprint string `json:"fingerprint"`
	Log         string `json:"log"`
}

// OnboardServiceUser connects with the given (one-off) credentials, installs the
// service account carrying the installation key, and returns what it created.
// It does not touch the config — the caller decides whether to save a router
// that now has a working service account.
func OnboardServiceUser(r config.Router, o DeployOptions, publicKey string) (ServiceUserResult, error) {
	c, addr, err := connect(r, o)
	if err != nil {
		return ServiceUserResult{}, err
	}
	defer c.Close()

	if err := c.EnsureServiceUser(publicKey); err != nil {
		return ServiceUserResult{Log: c.Transcript()}, err
	}
	st, err := c.ServiceUserStatus()
	if err != nil {
		return ServiceUserResult{Log: c.Transcript()}, err
	}
	return ServiceUserResult{
		Address:  addr,
		User:     deploy.ServiceUserName,
		Policies: st.Policies,
		Log:      c.Transcript(),
	}, nil
}

// OffboardServiceUser removes the service account from the router. It is called
// with the router's own stored credentials — the account deletes itself, so no
// administrator login is needed to hand a router back.
func OffboardServiceUser(cfg config.Config, routerName string, o DeployOptions) (ServiceUserResult, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return ServiceUserResult{}, err
	}
	c, addr, err := connect(r, o)
	if err != nil {
		return ServiceUserResult{}, fmt.Errorf("%w (the account stays on the router; remove it by hand: %s)",
			err, deploy.RemoveServiceUserCommand())
	}
	defer c.Close()
	if err := c.RemoveServiceUser(); err != nil {
		return ServiceUserResult{Log: c.Transcript()}, err
	}
	return ServiceUserResult{Router: routerName, Address: addr, User: deploy.ServiceUserName, Log: c.Transcript()}, nil
}

// RouterByName exposes a router entry for callers that need to onboard before
// the router is part of a saved config.
func RouterByName(cfg config.Config, name string) (config.Router, error) { return getRouter(cfg, name) }

// ServiceUserState reports the account's presence on a router.
func ServiceUserState(cfg config.Config, routerName string, o DeployOptions) (deploy.ServiceUserStatus, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return deploy.ServiceUserStatus{}, err
	}
	c, _, err := connect(r, o)
	if err != nil {
		return deploy.ServiceUserStatus{}, err
	}
	defer c.Close()
	return c.ServiceUserStatus()
}
