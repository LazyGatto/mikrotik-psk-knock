package desktopui

import (
	"fmt"
	"strconv"
	"strings"
)

// LaunchKey identifies one service's launch command in Settings.Launch.
func LaunchKey(inviteID, router, service string) string {
	return stateKey(inviteID, router, service)
}

// expandLaunch substitutes the placeholders a launch command may use. The
// values come from the invite the user already imported, never from user input
// at run time, so there is nothing to sanitize beyond the substitution itself.
//
//	{host}    router address
//	{port}    the service's check port (the port that just opened)
//	{service} service name
func expandLaunch(cmdline, host string, port int, service string) string {
	r := strings.NewReplacer(
		"{host}", host,
		"{port}", strconv.Itoa(port),
		"{service}", service,
	)
	return r.Replace(cmdline)
}

// launchCommand returns the user's command for a service, or "" when unset.
func (s *Server) launchCommand(inviteID, router, service string) string {
	st := s.store.Settings()
	if st.Launch == nil {
		return ""
	}
	return strings.TrimSpace(st.Launch[LaunchKey(inviteID, router, service)])
}

// SetLaunchCommand stores (or clears, when cmdline is empty) the user's launch
// command for one service.
func (s *Server) SetLaunchCommand(inviteID, router, service, cmdline string) error {
	if _, err := s.store.Get(inviteID); err != nil {
		return err
	}
	st := s.store.Settings()
	if st.Launch == nil {
		st.Launch = map[string]string{}
	}
	key := LaunchKey(inviteID, router, service)
	if strings.TrimSpace(cmdline) == "" {
		delete(st.Launch, key)
	} else {
		st.Launch[key] = strings.TrimSpace(cmdline)
	}
	if err := s.store.SaveSettings(st); err != nil {
		return fmt.Errorf("desktopui: save launch command: %w", err)
	}
	return nil
}
