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

// Brand tile for the About popup — a copy of cmd/mkpk-client/winres/icon.png
// (go:embed cannot reach outside the package dir; keep the two in sync).
//
//go:embed icon.png
var iconPNG []byte

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
	state   stateRegistry
	openURL func(string) error // shell hook: open a URL in the system browser
	runCmd  func(string) error // launch runner; swapped in tests
}

// RepoURL is the public project page (the About link target).
const RepoURL = "https://github.com/LazyGatto/mikrotik-psk-knock"

// SetOpenURL lends the shell's system-browser hook to the UI (webview links
// would navigate the app page away instead of opening a browser).
func (s *Server) SetOpenURL(f func(string) error) { s.openURL = f }

// NewHandler builds the handler with default knock timings.
func NewHandler(store *Store, sessionToken string) http.Handler {
	return NewHandlerTimings(store, sessionToken, defaultTimings())
}

// New builds the Server itself — the desktop shell needs it directly (tray:
// Status/Knock/SetOnChange), with Handler() mounted as the asset server.
func New(store *Store, sessionToken string, t KnockTimings) *Server {
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
	srv := &Server{store: store, token: sessionToken, timings: t}
	srv.runCmd = srv.runCommandLine
	return srv
}

// NewHandlerTimings builds the handler with explicit timings (tests).
func NewHandlerTimings(store *Store, sessionToken string, t KnockTimings) http.Handler {
	return New(store, sessionToken, t).Handler()
}

// Handler mounts the UI + API routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/api/state", s.auth(s.handleState))
	mux.HandleFunc("/api/import", s.auth(s.importInvite))
	mux.HandleFunc("/api/remove", s.auth(s.removeInvite))
	mux.HandleFunc("/api/knock", s.auth(s.handleKnock))
	mux.HandleFunc("/api/settings", s.auth(s.handleSettings))
	mux.HandleFunc("/api/open", s.auth(s.handleOpen))
	mux.HandleFunc("/api/launch", s.auth(s.handleLaunch))
	mux.HandleFunc("/api/relaunch", s.auth(s.handleRelaunch))
	mux.HandleFunc("/icon.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(iconPNG)
	})
	return mux
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page := strings.Replace(indexHTML, "__MKPK_TOKEN__", s.token, 1)
	page = strings.ReplaceAll(page, "__MKPK_VERSION__", version.String())
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
	Launch         string `json:"launch,omitempty"`      // user's local post-open command
	LaunchKind     string `json:"launch_kind,omitempty"` // admin's preset from the invite
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

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	st := s.store.Settings()
	invites := make([]inviteJSON, 0, len(list))
	for _, inv := range list {
		ij := toInviteJSON(inv)
		for ri, rt := range ij.Routers {
			for si, svc := range rt.Services {
				ij.Routers[ri].Services[si].Launch = st.Launch[LaunchKey(inv.ID, rt.Router, svc.Name)]
			}
		}
		invites = append(invites, ij)
	}
	writeJSON(w, map[string]any{
		"version":  version.String(),
		"language": st.Language,
		"theme":    st.Theme,
		"invites":  invites,
		// Where launches are recorded — the answer to "it says launched but
		// nothing happened".
		"launch_log": s.LaunchLogPath(),
	})
}

func toInviteJSON(inv StoredInvite) inviteJSON {
	out := inviteJSON{ID: inv.ID, ClientID: inv.Invite.ClientID}
	for _, rt := range inv.Invite.Routers {
		rj := routerJSON{Router: rt.Router}
		for _, svc := range rt.Services {
			rj.Services = append(rj.Services, serviceJSON{
				Name: svc.Name, CheckPort: svc.CheckPort, AllowedTimeout: svc.AllowedTimeout,
				LaunchKind: svc.Launch,
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

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		Language *string `json:"language"`
		Theme    *string `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request: "+err.Error())
		return
	}
	st := s.store.Settings()
	if req.Language != nil {
		if *req.Language != "en" && *req.Language != "ru" {
			writeErr(w, http.StatusBadRequest, "language must be en or ru")
			return
		}
		st.Language = *req.Language
	}
	if req.Theme != nil {
		if *req.Theme != "dark" && *req.Theme != "light" {
			writeErr(w, http.StatusBadRequest, "theme must be dark or light")
			return
		}
		st.Theme = *req.Theme
	}
	if err := s.store.SaveSettings(st); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"language": st.Language, "theme": st.Theme})
}

// handleOpen opens one of our project pages in the system browser — not a
// generic URL launcher.
func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.openURL == nil {
		writeErr(w, http.StatusBadRequest, "no browser hook in this mode")
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request: "+err.Error())
		return
	}
	if !strings.HasPrefix(req.URL, RepoURL) {
		writeErr(w, http.StatusBadRequest, "url outside the project pages")
		return
	}
	if err := s.openURL(req.URL); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleLaunch stores the user's post-open command for one service (empty
// clears it). The command is local to this machine and never leaves it.
func (s *Server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		InviteID string `json:"invite_id"`
		Router   string `json:"router"`
		Service  string `json:"service"`
		Command  string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request: "+err.Error())
		return
	}
	if err := s.SetLaunchCommand(req.InviteID, req.Router, req.Service, req.Command); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"command": strings.TrimSpace(req.Command)})
}

// KnockResult is the outcome of one knock+check run.
type KnockResult struct {
	Router     string `json:"router"`
	Service    string `json:"service"`
	Knocked    bool   `json:"knocked"`
	Status     string `json:"status"`
	Attempts   int    `json:"attempts"`
	DurationMS int64  `json:"duration_ms"`
	LastError  string `json:"error"`
	Launched   string `json:"launched,omitempty"`     // command started after open
	LaunchErr  string `json:"launch_error,omitempty"` // it failed to start
}

// Knock performs the full CLI knock flow for one service of one stored invite:
// wait out the bucket edge → compute the token → staged UDP knock → TCP check
// of the target port. Synchronous (seconds); shared by the HTTP API and the
// tray menu. A successful open is recorded in the state registry so the tray
// can show "open · Nm left".
func (s *Server) Knock(inviteID, routerAddr, serviceName string) (KnockResult, error) {
	inv, err := s.store.Get(inviteID)
	if err != nil {
		return KnockResult{}, err
	}
	rt, svc, err := findService(inv.Invite, routerAddr, serviceName)
	if err != nil {
		return KnockResult{}, err
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
		return KnockResult{}, fmt.Errorf("knock: %w", err)
	}
	chk := servicecheck.Check(servicecheck.Options{
		Host:     rt.Router,
		Port:     svc.CheckPort,
		Timeout:  s.timings.CheckTimeout,
		Attempts: s.timings.CheckAttempts,
		Interval: s.timings.CheckInterval,
	})
	res := KnockResult{
		Router: rt.Router, Service: svc.Name, Knocked: true,
		Status: chk.Status, Attempts: chk.Attempts,
		DurationMS: chk.Duration.Milliseconds(), LastError: chk.LastError,
	}
	if chk.Status != "open" {
		return res, nil // nothing is launched unless the port really opened
	}
	if svc.AllowedTimeout != "" {
		if d, err := parseGoDuration(svc.AllowedTimeout); err == nil {
			s.state.markOpen(inv.ID, rt.Router, svc.Name, time.Now().Add(d))
		}
	}
	cmdline, lerr := s.launchLine(inv.ID, rt, svc)
	if lerr != nil {
		res.LaunchErr = lerr.Error()
	}
	if cmdline != "" && s.runCmd != nil {
		if err := s.runCmd(cmdline); err != nil {
			res.LaunchErr = err.Error()
		} else {
			res.Launched = cmdline
		}
	}
	return res, nil
}

// launchLine decides what to run after a service is open: the user's own
// command wins; otherwise the invite's preset KIND, expanded by us into a
// platform invocation. An empty line means "nothing to run".
func (s *Server) launchLine(inviteID string, rt invite.Router, svc invite.Service) (string, error) {
	if line := s.launchCommand(inviteID, rt.Router, svc.Name); line != "" {
		return expandLaunch(line, rt.Router, svc.CheckPort, svc.Name), nil
	}
	if svc.Launch != "" {
		return presetLine(svc.Launch, rt.Router, svc.CheckPort)
	}
	return "", nil
}

// RelaunchResult is the outcome of re-running the command for a service that
// is already open.
type RelaunchResult struct {
	Router    string `json:"router"`
	Service   string `json:"service"`
	Status    string `json:"status"` // port check: open / closed / error
	Attempts  int    `json:"attempts"`
	LastError string `json:"error,omitempty"`
	Launched  string `json:"launched,omitempty"`
	LaunchErr string `json:"launch_error,omitempty"`
}

// Relaunch runs the service's command again without knocking. Reconnecting to
// a session that dropped should not cost a knock while the router still holds
// the window open — but the window may have expired meanwhile, so the port is
// checked first and a closed one is reported rather than launched into.
func (s *Server) Relaunch(inviteID, routerAddr, serviceName string) (RelaunchResult, error) {
	inv, err := s.store.Get(inviteID)
	if err != nil {
		return RelaunchResult{}, err
	}
	rt, svc, err := findService(inv.Invite, routerAddr, serviceName)
	if err != nil {
		return RelaunchResult{}, err
	}
	cmdline, lerr := s.launchLine(inv.ID, rt, svc)
	if cmdline == "" {
		if lerr != nil {
			return RelaunchResult{}, lerr
		}
		return RelaunchResult{}, fmt.Errorf("desktopui: no command set for %s", svc.Name)
	}

	// One attempt: this is a fast "is it still open?", not the patient wait a
	// knock does while the router installs the rule.
	chk := servicecheck.Check(servicecheck.Options{
		Host:     rt.Router,
		Port:     svc.CheckPort,
		Timeout:  s.timings.CheckTimeout,
		Attempts: 1,
		Interval: s.timings.CheckInterval,
	})
	res := RelaunchResult{
		Router: rt.Router, Service: svc.Name,
		Status: chk.Status, Attempts: chk.Attempts, LastError: chk.LastError,
	}
	if chk.Status != "open" {
		return res, nil // caller tells the user to knock again
	}
	if lerr != nil {
		res.LaunchErr = lerr.Error()
		return res, nil
	}
	if s.runCmd != nil {
		if err := s.runCmd(cmdline); err != nil {
			res.LaunchErr = err.Error()
		} else {
			res.Launched = cmdline
		}
	}
	return res, nil
}

func (s *Server) handleRelaunch(w http.ResponseWriter, r *http.Request) {
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
	res, err := s.Relaunch(req.InviteID, req.Router, req.Service)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleKnock(w http.ResponseWriter, r *http.Request) {
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
	res, err := s.Knock(req.InviteID, req.Router, req.Service)
	if err != nil {
		if strings.HasPrefix(err.Error(), "knock:") {
			writeErr(w, http.StatusBadGateway, err.Error())
		} else {
			writeErr(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, res)
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
