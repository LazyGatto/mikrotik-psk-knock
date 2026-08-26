package desktopui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mikrotik-psk-knock/client/internal/invite"
	"mikrotik-psk-knock/client/internal/knock"
	"mikrotik-psk-knock/client/internal/servicecheck"
	"mikrotik-psk-knock/client/internal/token"
	"mikrotik-psk-knock/client/internal/version"
)

//go:embed index.html
var indexHTML string

// KnockTimings groups the knock/check pacing. The zero value means CLI
// defaults; tests shrink them so the gate stays fast.
type KnockTimings struct {
	MinBucketAge  time.Duration // wait until the bucket is at least this old
	StageDuration time.Duration
	TokenDuration time.Duration
	CheckTimeout  time.Duration
	CheckAttempts int
	CheckInterval time.Duration
}

func defaultTimings() KnockTimings {
	return KnockTimings{
		MinBucketAge:  2 * time.Second,
		StageDuration: 2 * time.Second,
		TokenDuration: time.Second,
		CheckTimeout:  time.Second,
		CheckAttempts: 10,
		CheckInterval: 500 * time.Millisecond,
	}
}

// Server is the desktop client's embedded HTTP backend.
type Server struct {
	store   *Store
	token   string // per-session token, injected into the page, gates /api
	timings KnockTimings
}

// NewHandler builds the handler with default knock timings.
func NewHandler(store *Store, sessionToken string) http.Handler {
	return NewHandlerTimings(store, sessionToken, defaultTimings())
}

// NewHandlerTimings builds the handler with explicit timings (tests).
func NewHandlerTimings(store *Store, sessionToken string, t KnockTimings) http.Handler {
	d := defaultTimings()
	if t.MinBucketAge == 0 {
		t.MinBucketAge = d.MinBucketAge
	}
	if t.StageDuration == 0 {
		t.StageDuration = d.StageDuration
	}
	if t.TokenDuration == 0 {
		t.TokenDuration = d.TokenDuration
	}
	if t.CheckTimeout == 0 {
		t.CheckTimeout = d.CheckTimeout
	}
	if t.CheckAttempts == 0 {
		t.CheckAttempts = d.CheckAttempts
	}
	if t.CheckInterval == 0 {
		t.CheckInterval = d.CheckInterval
	}
	s := &Server{store: store, token: sessionToken, timings: t}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/api/state", s.auth(s.state))
	mux.HandleFunc("/api/import", s.auth(s.importInvite))
	mux.HandleFunc("/api/remove", s.auth(s.removeInvite))
	mux.HandleFunc("/api/knock", s.auth(s.knock))
	mux.HandleFunc("/api/language", s.auth(s.language))
	return mux
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page := strings.Replace(indexHTML, "__MKPK_TOKEN__", s.token, 1)
	page = strings.Replace(page, "__MKPK_VERSION__", version.String(), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// auth gates API calls with the session token, so only our own page can call
// them (same scheme as internal/web).
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MKPK-Token") != s.token {
			writeErr(w, http.StatusForbidden, "invalid session token")
			return
		}
		next(w, r)
	}
}

// --- API ---------------------------------------------------------------------

type serviceJSON struct {
	Name           string `json:"name"`
	CheckPort      int    `json:"check_port"`
	AllowedTimeout string `json:"allowed_timeout,omitempty"`
}

type routerJSON struct {
	Router   string        `json:"router"`
	Services []serviceJSON `json:"services"`
}

type inviteJSON struct {
	ID       string       `json:"id"`
	ClientID string       `json:"client_id"`
	Routers  []routerJSON `json:"routers"`
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	invites := make([]inviteJSON, 0, len(list))
	for _, inv := range list {
		invites = append(invites, toInviteJSON(inv))
	}
	writeJSON(w, map[string]any{
		"version":  version.String(),
		"language": s.store.Settings().Language,
		"invites":  invites,
	})
}

func toInviteJSON(inv StoredInvite) inviteJSON {
	out := inviteJSON{ID: inv.ID, ClientID: inv.Invite.ClientID}
	for _, rt := range inv.Invite.Routers {
		rj := routerJSON{Router: rt.Router}
		for _, svc := range rt.Services {
			rj.Services = append(rj.Services, serviceJSON{
				Name: svc.Name, CheckPort: svc.CheckPort, AllowedTimeout: svc.AllowedTimeout,
			})
		}
		out.Routers = append(out.Routers, rj)
	}
	return out
}

func (s *Server) importInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		Name string `json:"name"`
		Blob string `json:"blob"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request: "+err.Error())
		return
	}
	inv, err := s.store.Add(req.Name, req.Blob)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, toInviteJSON(inv))
}

func (s *Server) removeInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request: "+err.Error())
		return
	}
	if err := s.store.Remove(req.ID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"removed": req.ID})
}

func (s *Server) language(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request: "+err.Error())
		return
	}
	if req.Language != "en" && req.Language != "ru" {
		writeErr(w, http.StatusBadRequest, "language must be en or ru")
		return
	}
	st := s.store.Settings()
	st.Language = req.Language
	if err := s.store.SaveSettings(st); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"language": req.Language})
}

// knock performs the full CLI knock flow for one service of one stored invite:
// wait out the bucket edge → compute the token → staged UDP knock → TCP check
// of the target port. Synchronous; the page shows progress client-side.
func (s *Server) knock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		InviteID string `json:"invite_id"`
		Router   string `json:"router"`
		Service  string `json:"service"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request: "+err.Error())
		return
	}
	inv, err := s.store.Get(req.InviteID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rt, svc, err := findService(inv.Invite, req.Router, req.Service)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Do not send a token right after a bucket edge: the router's poller may
	// still be on the previous bucket (same rule as the CLI's --min-bucket-age).
	now := time.Now()
	if age := token.InspectWindow(now, rt.BucketSeconds).Age; age < s.timings.MinBucketAge {
		time.Sleep(s.timings.MinBucketAge - age)
		now = time.Now()
	}
	bucket := token.Bucket(now, rt.BucketSeconds)
	value := token.Compute(rt.PSK, svc.Name, inv.Invite.ClientID, bucket)

	if err := knock.Run(knock.Options{
		Router:        rt.Router,
		Stage1Port:    svc.Stage1,
		Stage2Port:    svc.Stage2,
		TokenPort:     svc.Token,
		Token:         value,
		StageDuration: s.timings.StageDuration,
		TokenDuration: s.timings.TokenDuration,
	}); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	chk := servicecheck.Check(servicecheck.Options{
		Host:     rt.Router,
		Port:     svc.CheckPort,
		Timeout:  s.timings.CheckTimeout,
		Attempts: s.timings.CheckAttempts,
		Interval: s.timings.CheckInterval,
	})
	writeJSON(w, map[string]any{
		"router": rt.Router, "service": svc.Name, "knocked": true,
		"status": chk.Status, "attempts": chk.Attempts,
		"duration_ms": chk.Duration.Milliseconds(),
		"error":       chk.LastError,
	})
}

func findService(b invite.Blob, routerAddr, serviceName string) (invite.Router, invite.Service, error) {
	for _, rt := range b.Routers {
		if rt.Router != routerAddr {
			continue
		}
		for _, svc := range rt.Services {
			if svc.Name == serviceName {
				return rt, svc, nil
			}
		}
		return invite.Router{}, invite.Service{}, fmt.Errorf("desktopui: service %q not in invite router %q", serviceName, routerAddr)
	}
	return invite.Router{}, invite.Service{}, fmt.Errorf("desktopui: router %q not in invite", routerAddr)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
