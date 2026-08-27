package desktopui

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// Keep-open: hold a service open by re-knocking shortly before the router's
// allowed-timeout runs out, instead of making the user press Knock every few
// minutes. Ported from the native macOS client so both recipients behave the
// same way — same lead time, same give-up rule.
//
// The router is the authority on the real TTL; the countdown here is derived
// from the invite's allowed_timeout and is informational.

// keepOpenTick is how often the maintainer looks at the world. Renewal itself
// only fires near expiry, so this cheap tick costs nothing.
const keepOpenTick = 5 * time.Second

// keepOpenFailLimit is how many consecutive failed renewals turn the switch
// off. Three is the macOS client's rule: enough to ride out a flaky link,
// little enough that a genuinely unreachable router stops being knocked at.
const keepOpenFailLimit = 3

// renewLead is how long before expiry to re-knock: one bucket, clamped. A knock
// lands in the *current* bucket, so leaving less than a bucket risks the router
// having already dropped the entry.
func renewLead(bucketSeconds int64) time.Duration {
	lead := time.Duration(bucketSeconds) * time.Second
	if lead < 15*time.Second {
		lead = 15 * time.Second
	}
	if lead > 30*time.Second {
		lead = 30 * time.Second
	}
	return lead
}

type keepOpenState struct {
	mu       sync.Mutex
	inFlight map[string]bool
	failures map[string]int
}

func (k *keepOpenState) begin(key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.inFlight == nil {
		k.inFlight = map[string]bool{}
	}
	if k.inFlight[key] {
		return false
	}
	k.inFlight[key] = true
	return true
}

func (k *keepOpenState) end(key string) {
	k.mu.Lock()
	delete(k.inFlight, key)
	k.mu.Unlock()
}

// fail counts a failure and reports whether the limit is reached.
func (k *keepOpenState) fail(key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.failures == nil {
		k.failures = map[string]int{}
	}
	k.failures[key]++
	return k.failures[key] >= keepOpenFailLimit
}

func (k *keepOpenState) succeed(key string) {
	k.mu.Lock()
	delete(k.failures, key)
	k.mu.Unlock()
}

// KeepOpen reports whether the user asked to hold this service open.
func (s *Server) KeepOpen(inviteID, router, service string) bool {
	st := s.store.Settings()
	return st.KeepOpen[LaunchKey(inviteID, router, service)]
}

// SetKeepOpen turns holding on or off for one service. Turning it on knocks
// straight away when the port is not already known to be open — otherwise the
// switch would appear to do nothing until the current window expired.
func (s *Server) SetKeepOpen(inviteID, router, service string, on bool) error {
	if _, err := s.store.Get(inviteID); err != nil {
		return err
	}
	st := s.store.Settings()
	if st.KeepOpen == nil {
		st.KeepOpen = map[string]bool{}
	}
	key := LaunchKey(inviteID, router, service)
	if on {
		st.KeepOpen[key] = true
	} else {
		delete(st.KeepOpen, key)
	}
	if err := s.store.SaveSettings(st); err != nil {
		return err
	}
	s.keep.succeed(key)
	if on && !s.state.isOpen(inviteID, router, service) {
		go s.renew(inviteID, router, service)
	}
	return nil
}

// StartKeepOpen runs the maintainer until stop is closed. The desktop shell
// calls it once; tests drive renew() directly.
func (s *Server) StartKeepOpen(stop <-chan struct{}) {
	go func() {
		tick := time.NewTicker(keepOpenTick)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				s.maintainKeepOpen()
			}
		}
	}()
}

// maintainKeepOpen re-knocks the services that need it: those whose window is
// about to end, and those that are not open at all (a laptop that slept through
// the expiry, a router that was briefly unreachable).
func (s *Server) maintainKeepOpen() {
	states, err := s.Status()
	if err != nil {
		return
	}
	settings := s.store.Settings()
	now := time.Now()
	for _, st := range states {
		key := LaunchKey(st.InviteID, st.Router, st.Service)
		if !settings.KeepOpen[key] {
			continue
		}
		if st.OpenUntil.IsZero() {
			go s.renew(st.InviteID, st.Router, st.Service)
			continue
		}
		lead := renewLead(s.bucketSeconds(st.InviteID, st.Router))
		if now.After(st.OpenUntil.Add(-lead)) {
			go s.renew(st.InviteID, st.Router, st.Service)
		}
	}
}

// bucketSeconds reads the router's bucket from the invite; 30 is the default
// everything else in the project uses.
func (s *Server) bucketSeconds(inviteID, routerAddr string) int64 {
	inv, err := s.store.Get(inviteID)
	if err != nil {
		return 30
	}
	for _, rt := range inv.Invite.Routers {
		if rt.Router == routerAddr && rt.BucketSeconds > 0 {
			return rt.BucketSeconds
		}
	}
	return 30
}

// renew performs a silent knock: no launch command, no UI toast — the user
// asked for the port to stay open, not for something to happen. Three failures
// in a row switch keep-open off rather than knocking at an unreachable router
// forever.
func (s *Server) renew(inviteID, router, service string) {
	key := LaunchKey(inviteID, router, service)
	if !s.begin(key) {
		return
	}
	defer s.keep.end(key)

	res, err := s.knock(inviteID, router, service, false)
	if err == nil && res.Status == "open" {
		s.keep.succeed(key)
		return
	}
	if !s.keep.fail(key) {
		return
	}
	// Give up: turn the switch off so the UI stops claiming it is being held.
	st := s.store.Settings()
	if st.KeepOpen != nil {
		delete(st.KeepOpen, key)
		if saveErr := s.store.SaveSettings(st); saveErr != nil {
			log.Printf("desktopui: keep-open off for %s: %v", key, saveErr)
		}
	}
	s.keep.succeed(key) // reset the counter for a later manual retry
	s.state.markClosed(inviteID, router, service)
	log.Printf("desktopui: keep-open stopped for %s after %d failures", key, keepOpenFailLimit)
}

// begin guards against overlapping renewals of the same service.
func (s *Server) begin(key string) bool { return s.keep.begin(key) }

func (s *Server) handleKeepOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		InviteID string `json:"invite_id"`
		Router   string `json:"router"`
		Service  string `json:"service"`
		On       bool   `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request: "+err.Error())
		return
	}
	if err := s.SetKeepOpen(req.InviteID, req.Router, req.Service, req.On); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"keep_open": req.On})
}

// handleStatus is the page's cheap poll: just the live bits, so a countdown can
// tick and a background renewal shows up without re-rendering the whole list
// (which would close whatever editor the user has open).
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	states, err := s.Status()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings := s.store.Settings()
	out := map[string]any{}
	for _, st := range states {
		key := LaunchKey(st.InviteID, st.Router, st.Service)
		row := map[string]any{"keep_open": settings.KeepOpen[key]}
		if !st.OpenUntil.IsZero() {
			row["open_until"] = st.OpenUntil.Unix()
		}
		out[key] = row
	}
	writeJSON(w, map[string]any{"now": time.Now().Unix(), "services": out})
}
