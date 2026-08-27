package desktopui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ServiceState is a snapshot row for the tray / UI: one knockable service and,
// when a knock succeeded and its allowed-timeout is known, until when the
// router keeps it open. Informational — the router enforces the real TTL.
type ServiceState struct {
	InviteID  string
	Router    string
	Service   string
	OpenUntil time.Time // zero → not known to be open
}

// Open reports whether the entry is still within its allowed window.
func (s ServiceState) Open() bool { return time.Now().Before(s.OpenUntil) }

type stateRegistry struct {
	mu       sync.Mutex
	openTill map[string]time.Time // inviteID/router/service → open until
	onChange func()
}

func stateKey(inviteID, router, service string) string {
	return inviteID + "/" + router + "/" + service
}

func (r *stateRegistry) markOpen(inviteID, router, service string, until time.Time) {
	r.mu.Lock()
	if r.openTill == nil {
		r.openTill = map[string]time.Time{}
	}
	r.openTill[stateKey(inviteID, router, service)] = until
	cb := r.onChange
	r.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// markClosed forgets an open window: the port is no longer known to be open.
func (r *stateRegistry) markClosed(inviteID, router, service string) {
	r.mu.Lock()
	delete(r.openTill, stateKey(inviteID, router, service))
	cb := r.onChange
	r.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// isOpen reports whether the service is currently within a known window.
func (r *stateRegistry) isOpen(inviteID, router, service string) bool {
	return time.Now().Before(r.openUntil(inviteID, router, service))
}

func (r *stateRegistry) openUntil(inviteID, router, service string) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.openTill[stateKey(inviteID, router, service)]
}

// SetOnChange registers a callback fired after every state change (tray).
func (s *Server) SetOnChange(f func()) {
	s.state.mu.Lock()
	s.state.onChange = f
	s.state.mu.Unlock()
}

// Status returns one row per knockable service across all stored invites,
// with the open-until mark where a successful knock is still in effect.
func (s *Server) Status() ([]ServiceState, error) {
	invites, err := s.store.List()
	if err != nil {
		return nil, err
	}
	var out []ServiceState
	for _, inv := range invites {
		for _, rt := range inv.Invite.Routers {
			for _, svc := range rt.Services {
				out = append(out, ServiceState{
					InviteID:  inv.ID,
					Router:    rt.Router,
					Service:   svc.Name,
					OpenUntil: s.state.openUntil(inv.ID, rt.Router, svc.Name),
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Router != b.Router {
			return a.Router < b.Router
		}
		return a.Service < b.Service
	})
	return out, nil
}

// parseGoDuration parses the invite's allowed_timeout ("55m", "1h30m", "2d",
// "1w"): time.ParseDuration plus RouterOS-style d/w units.
func parseGoDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// Expand d/w into hours, then retry: "1d2h" → "24h2h" works because
	// time.ParseDuration sums repeated units.
	var sb strings.Builder
	num := ""
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9' || r == '.':
			num += string(r)
		case r == 'd' || r == 'w':
			n, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, fmt.Errorf("bad duration %q", s)
			}
			hours := n * 24
			if r == 'w' {
				hours *= 7
			}
			fmt.Fprintf(&sb, "%gh", hours)
			num = ""
		default:
			sb.WriteString(num)
			sb.WriteRune(r)
			num = ""
		}
	}
	sb.WriteString(num)
	return time.ParseDuration(sb.String())
}
