// Package web serves a local (127.0.0.1) admin UI over the internal/admin core:
// view and edit the config, render the .rsc and deploy over SSH. It is a thin
// HTTP frontend — all real work is in internal/admin. A per-session token gates
// the API so other-origin pages in the browser cannot drive it.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mikrotik-psk-knock/client/internal/admin"
	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/deploy"
	"mikrotik-psk-knock/client/internal/version"
)

//go:embed assets/login.html assets/index.html assets/app.js assets/style.css assets/favicon-16.png assets/favicon-32.png assets/apple-touch-icon.png assets/logo-96.png assets/icon-512.png
var assetsFS embed.FS

const maxUndo = 100

// Server holds the config path, session token and the undo/redo history. History
// is per running session, in memory: raw config-file snapshots (the config is
// tiny). A single local operator uses it, so a mutex is enough for safety.
type Server struct {
	configPath string
	token      string
	desktop    bool // desktop build: browser blob-downloads don't work, save server-side
	hooks      DesktopHooks
	auth       *Auth // nil → local mode: loopback + a static per-process token

	updMu      sync.Mutex
	updCache   updateInfo
	updFetched time.Time
	mu         sync.Mutex
	undo       [][]byte // snapshots of the file before each mutation
	redo       [][]byte
}

// Handler builds the HTTP handler for the local admin UI served over a loopback
// TCP listener (`mkpk-provision serve`). The Host guard blocks DNS-rebinding
// against that listener.
func Handler(configPath, token string) http.Handler {
	return loopbackOnly(mux(configPath, token, false, DesktopHooks{}))
}

// AuthHandler builds the UI for a networked, shared instance: every request is
// gated by a session from the shared admin password. No loopback guard — the
// point is to reach it over the network — and no static token: the per-session
// token injected into the page doubles as the CSRF token.
func AuthHandler(configPath string, auth *Auth) http.Handler {
	s := &Server{configPath: configPath, auth: auth}
	return s.routes()
}

// EmbeddedHandler builds the same admin UI for the desktop app, where it is
// mounted directly as the Wails asset server. There is no TCP listener to rebind
// against, and the webview's Host header is platform-specific, so the loopback
// Host guard is omitted; the per-session token still gates the API.
func EmbeddedHandler(configPath, token string) http.Handler {
	return mux(configPath, token, true, DesktopHooks{})
}

// DesktopHooks are native capabilities the desktop shell can lend to the UI.
// Both are optional; absent hooks fall back to the browser-era behavior.
type DesktopHooks struct {
	// SaveDialog shows a native save dialog and returns the chosen path
	// ("" → the user cancelled).
	SaveDialog func(filename string) (string, error)
	// OpenURL opens a URL in the system browser (webview links would navigate
	// the app page away instead).
	OpenURL func(url string) error
}

// EmbeddedHandlerHooks is EmbeddedHandler with native desktop capabilities.
func EmbeddedHandlerHooks(configPath, token string, hooks DesktopHooks) http.Handler {
	return mux(configPath, token, true, hooks)
}

// mux wires the routes shared by both entry points.
func mux(configPath, token string, desktop bool, hooks DesktopHooks) *http.ServeMux {
	s := &Server{configPath: configPath, token: token, desktop: desktop, hooks: hooks}
	return s.routes()
}

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	// Liveness probe for the UI heartbeat: no auth, no request-log noise (path is
	// outside /api/), so the loaded page can tell when serve is gone.
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/app.js", s.static("assets/app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("/style.css", s.static("assets/style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/favicon.ico", s.static("assets/favicon-32.png", "image/png"))
	mux.HandleFunc("/favicon-16.png", s.static("assets/favicon-16.png", "image/png"))
	mux.HandleFunc("/favicon-32.png", s.static("assets/favicon-32.png", "image/png"))
	mux.HandleFunc("/apple-touch-icon.png", s.static("assets/apple-touch-icon.png", "image/png"))
	mux.HandleFunc("/logo-96.png", s.static("assets/logo-96.png", "image/png"))
	mux.HandleFunc("/icon-512.png", s.static("assets/icon-512.png", "image/png"))
	mux.HandleFunc("/api/config", s.guard(s.handleConfig))
	mux.HandleFunc("/api/secret", s.guard(s.handleSecret))
	mux.HandleFunc("/api/ports/suggest", s.guard(s.handlePortsSuggest))
	mux.HandleFunc("/api/router", s.guard(s.versioned(s.handleRouter)))
	mux.HandleFunc("/api/router/info", s.guard(s.handleRouterInfo))
	mux.HandleFunc("/api/service", s.guard(s.versioned(s.handleService)))
	mux.HandleFunc("/api/service/enable", s.guard(s.versioned(s.handleServiceEnable)))
	mux.HandleFunc("/api/service/test/stream", s.guard(s.handleServiceTestStream))
	mux.HandleFunc("/api/note", s.guard(s.versioned(s.handleNote)))
	mux.HandleFunc("/api/save", s.guard(s.handleSave))
	mux.HandleFunc("/api/update", s.guard(s.handleUpdate))
	mux.HandleFunc("/api/open", s.guard(s.handleOpen))
	mux.HandleFunc("/api/client", s.guard(s.versioned(s.handleClient)))
	mux.HandleFunc("/api/user", s.guard(s.versioned(s.handleUser)))
	mux.HandleFunc("/api/user/psk", s.guard(s.versioned(s.handleUserPSK)))
	mux.HandleFunc("/api/undo", s.guard(s.versioned(s.handleUndo)))
	mux.HandleFunc("/api/redo", s.guard(s.versioned(s.handleRedo)))
	mux.HandleFunc("/api/export", s.guard(s.handleExport))
	mux.HandleFunc("/api/render", s.guard(s.handleRender))
	mux.HandleFunc("/api/deploy/status", s.guard(s.handleDeployStatus))
	mux.HandleFunc("/api/deploy/apply", s.guard(s.handleDeployApply))
	mux.HandleFunc("/api/deploy/uninstall", s.guard(s.handleDeployUninstall))
	mux.HandleFunc("/api/deploy/stream", s.guard(s.handleDeployStream))
	mux.HandleFunc("/api/sshkey", s.guard(s.handleSSHKey))
	mux.HandleFunc("/api/router/onboard", s.guard(s.handleOnboard))
	if s.auth != nil {
		mux.HandleFunc("/login", s.handleLogin)
		mux.HandleFunc("/logout", s.handleLogout)
		mux.HandleFunc("/api/password", s.guard(s.handlePassword))
	}
	return mux
}

// pkgLogger is where the package writes things that are not a request line —
// mainly deploy outcomes, which the request log cannot show: the deploy stream
// answers 200 and carries any failure inside its body, so a container log full
// of "POST /api/deploy/stream -> 200" looked like a healthy instance while the
// operator was staring at red text in the browser.
var pkgLogger atomic.Pointer[log.Logger]

func logf(format string, args ...any) {
	if l := pkgLogger.Load(); l != nil {
		l.Printf(format, args...)
	}
}

// debugLog turns off every bit of noise suppression below.
func debugLog() bool { return os.Getenv("MKPK_DEBUG") != "" }

// pollPaths are polled by the open page on a timer. At one line every 20s they
// bury everything worth reading, so they are logged only when they fail, crawl,
// or MKPK_DEBUG is set.
var pollPaths = map[string]bool{
	"/api/router/info": true,
	"/api/config":      true,
	"/api/update":      true,
}

// LogRequests wraps h to log each request (method, path, status, duration). API
// endpoints only — static asset noise is skipped. Failures carry the error
// message the client was given, so "why did that 400?" is answerable from the
// container log alone. It also installs l as the package logger.
func LogRequests(h http.Handler, l *log.Logger) http.Handler {
	if l != nil {
		pkgLogger.Store(l)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			return
		}
		took := time.Since(start)
		if pollPaths[r.URL.Path] && rec.status < 400 && took < 2*time.Second && !debugLog() {
			return
		}
		line := fmt.Sprintf("%s %s%s -> %d (%s)", r.Method, r.URL.Path, querySuffix(r), rec.status, took.Truncate(time.Millisecond))
		if msg := rec.errMessage(); msg != "" {
			line += ": " + msg
		}
		l.Print(line)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	errBuf []byte // body of a failed response, capped — it is a JSON error object
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status >= 400 && len(r.errBuf) < 512 {
		r.errBuf = append(r.errBuf, b[:min(len(b), 512-len(r.errBuf))]...)
	}
	return r.ResponseWriter.Write(b)
}

// errMessage digs the human-readable part out of a captured error body.
func (r *statusRecorder) errMessage() string {
	if len(r.errBuf) == 0 {
		return ""
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(r.errBuf, &body); err == nil && body.Error != "" {
		return body.Error
	}
	return strings.TrimSpace(string(r.errBuf))
}

// Flush forwards to the underlying writer so streaming handlers (deploy stream)
// keep working through the logging wrapper.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func querySuffix(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}

// loopbackOnly rejects requests whose Host is not localhost, which blocks
// DNS-rebinding attacks against the local server. Only the TCP-served mode
// (Handler) uses it; the desktop's in-process EmbeddedHandler has no listener to
// rebind against.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		switch host {
		case "localhost", "127.0.0.1", "::1", "[::1]":
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "forbidden host", http.StatusForbidden)
		}
	})
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	pageToken := s.token
	if s.auth != nil {
		sess, ok := s.auth.Session(r)
		if !ok {
			s.loginPage(w, "")
			return
		}
		pageToken = sess.csrf
	}
	data, err := assetsFS.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := strings.Replace(string(data), "__MKPK_TOKEN__", pageToken, 1)
	page = strings.Replace(page, "__MKPK_VERSION__", version.String(), 1)
	page = strings.Replace(page, "__MKPK_DESKTOP__", strconv.FormatBool(s.desktop), 1)
	page = strings.Replace(page, "__MKPK_AUTH__", strconv.FormatBool(s.auth != nil), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// handleOnboard installs mkpk's own service account on a router using
// administrator credentials supplied for this one call. Those credentials are
// never written to the config: what gets stored afterwards is the service
// account plus the instance key.
func (s *Server) handleOnboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Address  string `json:"address"`  // public/management address to reach now
		Port     int    `json:"port"`     // 0 → default
		User     string `json:"user"`     // administrator login
		Password string `json:"password"` // administrator password …
		KeyPath  string `json:"key_path"` // … or a key
		KeyPass  string `json:"key_pass"`
		UseAgent bool   `json:"use_agent"`
		// Tidy up after ourselves: an admin who set password-authentication=yes
		// to let us in should not have to remember to set it back.
		RestorePasswordAuth bool `json:"restore_password_auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Address) == "" {
		writeErr(w, http.StatusBadRequest, "router address is required")
		return
	}

	// The account carries this installation's key, so make sure there is one.
	key, err := admin.ReadInstanceKey(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !key.Exists {
		if key, err = admin.CreateInstanceKey(s.configPath, "mkpk-provision", false); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	res, err := admin.OnboardServiceUser(
		config.Router{Address: req.Address, Deploy: config.Deploy{Port: req.Port}},
		admin.DeployOptions{
			Address: req.Address, Port: req.Port,
			Auth: deploy.Auth{
				User: req.User, Password: req.Password,
				KeyPath: req.KeyPath, KeyPass: req.KeyPass, UseAgent: req.UseAgent,
			},
		}, key.PublicKey, req.RestorePasswordAuth)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "log": res.Log})
		return
	}
	res.Fingerprint = key.Fingerprint
	writeJSON(w, http.StatusOK, map[string]any{
		"user": res.User, "policies": res.Policies, "fingerprint": res.Fingerprint,
		"key_path": key.Path, "log": res.Log,
		"password_auth_before": res.PasswordAuthBefore,
		"password_auth_after":  res.PasswordAuthAfter,
	})
}

// handleSSHKey reports the instance deploy key and, on POST, creates it. Only
// the public half is ever served — the private key stays on this machine.
func (s *Server) handleSSHKey(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		info, err := admin.ReadInstanceKey(s.configPath)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, info)
	case http.MethodPost:
		var req struct {
			Replace bool   `json:"replace"`
			Comment string `json:"comment"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req) // empty body → create if absent
		info, err := admin.CreateInstanceKey(s.configPath, req.Comment, req.Replace)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, info)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// --- authentication endpoints (shared-instance mode only) --------------------

func (s *Server) loginPage(w http.ResponseWriter, errMsg string) {
	data, err := assetsFS.ReadFile("assets/login.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := strings.Replace(string(data), "__MKPK_ERROR__", errMsg, 1)
	page = strings.Replace(page, "__MKPK_VERSION__", version.String(), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	_, _ = w.Write([]byte(page))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, ok := s.auth.Session(r); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		s.loginPage(w, "")
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.loginPage(w, "bad request")
		return
	}
	id, _, err := s.auth.Login(r.PostFormValue("password"))
	if err != nil {
		s.loginPage(w, "wrong password")
		return
	}
	s.auth.SetCookie(w, id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.Logout(r)
	s.auth.ClearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if err := s.auth.ChangePassword(r, req.Current, req.Next); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) static(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := assetsFS.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(data)
	}
}

// guard gates API calls, so only our own page can call them.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := s.token
		if s.auth != nil {
			// Shared instance: the caller must hold a live session, and the
			// page token bound to it. A custom header cannot be sent
			// cross-site without a CORS preflight, so this doubles as CSRF
			// protection alongside the SameSite=Strict cookie.
			sess, ok := s.auth.Session(r)
			if !ok {
				writeErr(w, http.StatusUnauthorized, "not signed in")
				return
			}
			token = sess.csrf
		}
		if r.Header.Get("X-MKPK-Token") != token {
			writeErr(w, http.StatusForbidden, "invalid session token")
			return
		}
		next(w, r)
	}
}

// versioned rejects a mutation aimed at a config that has changed underneath
// the caller — the lost-update guard for a shared instance. Requests that carry
// no version header (scripts, older pages) are let through unchanged.
func (s *Server) versioned(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if want := r.Header.Get("X-MKPK-Config-Version"); want != "" {
			if got := configVersion(s.configPath); got != "" && got != want {
				writeErr(w, http.StatusConflict,
					"the config changed in another session — reload before saving")
				return
			}
		}
		next(w, r)
	}
}

// configVersion is a content hash of the config file; "" when it cannot be read
// (a missing file is a valid starting state and must not block the first save).
func configVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// --- config ---

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
}

func (s *Server) handleSecret(w http.ResponseWriter, r *http.Request) {
	secret, err := admin.GenerateSecret(32)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret})
}

// handlePortsSuggest returns free knock ports for the router so the UI can
// random-fill stage1/stage2/token without colliding with existing services.
func (s *Server) handlePortsSuggest(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	count := 3
	if c := r.URL.Query().Get("count"); c != "" {
		if n, err := strconv.Atoi(c); err == nil {
			count = n
		}
	}
	ports, err := admin.SuggestPorts(cfg, r.URL.Query().Get("router"), count)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ports": ports})
}

type routerReq struct {
	Name           string        `json:"name"`
	Rename         string        `json:"rename"` // on edit: new name for the router
	Address        string        `json:"address"`
	DeployAddress  string        `json:"deploy_address"` // optional SSH override
	Port           int           `json:"port"`
	User           string        `json:"user"`
	KeyPath        string        `json:"key_path"`
	KeyPass        string        `json:"key_pass"`
	UseAgent       bool          `json:"use_agent"`
	Password       string        `json:"password"`
	Notify         config.Notify `json:"notify"`
	AllowedTimeout string        `json:"allowed_timeout"` // router-wide default allowed-list TTL
}

// handleServiceTestStream runs an end-to-end knock test for one (router, service,
// client) and streams progress as newline-delimited JSON ({type:log|result|error}),
// mirroring the deploy stream. It knocks the real router and reads its firewall
// state over SSH.
func (s *Server) handleServiceTestStream(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Router  string `json:"router"`
		Service string `json:"service"`
		Client  string `json:"client"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	emit := func(v any) {
		_ = enc.Encode(v)
		if flusher != nil {
			flusher.Flush()
		}
	}
	logf := func(format string, a ...any) {
		emit(map[string]any{"type": "log", "line": fmt.Sprintf(format, a...)})
	}
	opts := admin.DeployOptions{OnLog: func(line string) {
		emit(map[string]any{"type": "log", "line": line})
	}}
	res, err := admin.KnockTest(cfg, req.Router, req.Service, req.Client, 0, opts, logf)
	if err != nil {
		emit(map[string]any{"type": "error", "msg": err.Error()})
		return
	}
	emit(map[string]any{"type": "result", "result": res})
}

// handleRouterInfo returns a live health snapshot (device info + install state)
// for the router named in ?router=. The UI polls it periodically.
func (s *Server) handleRouterInfo(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := admin.RouterInfo(cfg, r.URL.Query().Get("router"), admin.DeployOptions{})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRouter(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req routerReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}
		dep := config.Deploy{
			Address: req.DeployAddress, Port: req.Port, User: req.User, KeyPath: req.KeyPath,
			KeyPass: req.KeyPass, UseAgent: req.UseAgent, Password: req.Password,
		}
		notify := req.Notify
		// Secrets are never sent to the browser, so a blank secret on edit means
		// "keep the stored one" rather than "clear it".
		if existing, ok := cfg.Routers[req.Name]; ok {
			if dep.Password == "" {
				dep.Password = existing.Deploy.Password
			}
			if dep.KeyPass == "" {
				dep.KeyPass = existing.Deploy.KeyPass
			}
			if notify.Telegram.BotToken == "" {
				notify.Telegram.BotToken = existing.Notify.Telegram.BotToken
			}
			if notify.Email.Password == "" {
				notify.Email.Password = existing.Notify.Email.Password
			}
		}
		target := req.Name
		if req.Rename != "" && req.Rename != req.Name {
			if cfg, err = admin.RenameRouter(cfg, req.Name, req.Rename); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			target = req.Rename
		}
		cfg, err = admin.SetRouter(cfg, admin.RouterOptions{Name: target, Address: req.Address, Deploy: dep, Notify: notify, AllowedTimeout: req.AllowedTimeout})
	case http.MethodDelete:
		// Removing a router should also hand it back: the service account
		// deletes itself, so no administrator credentials are needed. A router
		// we cannot reach is still removed from the config — with a warning, so
		// nobody is left believing the account is gone when it is not.
		name := r.URL.Query().Get("name")
		warning := ""
		if _, offErr := admin.OffboardServiceUser(cfg, name, admin.DeployOptions{}); offErr != nil {
			warning = offErr.Error()
		}
		cfg, err = admin.RemoveRouter(cfg, name)
		if err == nil && warning != "" {
			if saveErr := s.save(cfg); saveErr != nil {
				writeErr(w, http.StatusBadRequest, saveErr.Error())
				return
			}
			cfg, _ = config.LoadOrEmpty(s.configPath)
			s.writeConfigWarning(w, cfg, warning)
			return
		}
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.saveAndRespond(w, cfg)
}

type enableReq struct {
	Router  string `json:"router"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func (s *Server) handleServiceEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req enableReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	cfg, err = admin.SetServiceEnabled(cfg, req.Router, req.Name, req.Enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.saveAndRespond(w, cfg)
}

type noteReq struct {
	Kind   string `json:"kind"`   // "router" | "service" | "user"
	Router string `json:"router"` // service only
	Name   string `json:"name"`   // router / service / user name
	Note   string `json:"note"`
}

// handleNote updates the free-form local note on an entity. The note is stored
// only in this config — never rendered or exported — so it does not touch a
// router's render hash / deploy state.
func (s *Server) handleNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req noteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	cfg, err = admin.SetNote(cfg, req.Kind, req.Router, req.Name, req.Note)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.saveAndRespond(w, cfg)
}

type saveReq struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// handleSave writes a file to the user's Downloads directory. It exists for the
// desktop build, where the webview can't perform a browser blob-download; the app
// is local, so the in-process server writes the file directly and returns the
// path. The browser (serve) uses a normal download and doesn't call this.
func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req saveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	name := filepath.Base(strings.TrimSpace(req.Filename)) // strip any path — no traversal
	if name == "" || name == "." || name == string(filepath.Separator) {
		writeErr(w, http.StatusBadRequest, "invalid filename")
		return
	}
	var path string
	if s.hooks.SaveDialog != nil {
		chosen, err := s.hooks.SaveDialog(name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if chosen == "" { // user cancelled the dialog — not an error
			writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
			return
		}
		path = chosen
	} else {
		path = filepath.Join(downloadsDir(), name)
	}
	if err := os.WriteFile(path, []byte(req.Content), 0o644); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// downloadsDir returns the user's Downloads directory, falling back to home then
// the working directory.
func downloadsDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		if d := filepath.Join(home, "Downloads"); dirExists(d) {
			return d
		}
		return home
	}
	return "."
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	// router empty → bundle all of the user's routers into one blob.
	blob, err := admin.ExportUser(cfg, q.Get("user"), q.Get("router"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"blob": blob})
}

type serviceReq struct {
	Router         string        `json:"router"`
	Name           string        `json:"name"`
	Rename         string        `json:"rename"` // on edit: new name for the service
	ServiceName    string        `json:"service_name"`
	Disabled       bool          `json:"disabled"`
	Stage1Port     int           `json:"stage1_port"`
	Stage2Port     int           `json:"stage2_port"`
	TokenPort      int           `json:"token_port"`
	AllowedList    string        `json:"allowed_list"`
	AllowedTimeout string        `json:"allowed_timeout"`
	Target         config.Target `json:"target"`
	Launch         string        `json:"launch"`
	Force          bool          `json:"force"`
}

func (s *Server) handleService(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req serviceReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}
		target := req.Name
		if req.Rename != "" && req.Rename != req.Name {
			if cfg, err = admin.RenameService(cfg, req.Router, req.Name, req.Rename); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			target = req.Rename
		}
		cfg, err = admin.AddService(cfg, req.Router, admin.ServiceOptions{
			Name: target, ServiceName: req.ServiceName, Disabled: req.Disabled,
			Stage1Port: req.Stage1Port, Stage2Port: req.Stage2Port, TokenPort: req.TokenPort,
			AllowedList: req.AllowedList, AllowedTimeout: req.AllowedTimeout, Target: req.Target,
			Launch: req.Launch, Force: true,
		})
	case http.MethodDelete:
		q := r.URL.Query()
		cfg, err = admin.RemoveService(cfg, q.Get("router"), q.Get("name"))
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.saveAndRespond(w, cfg)
}

type clientReq struct {
	Router   string   `json:"router"`
	Name     string   `json:"name"`
	ClientID string   `json:"client_id"`
	Services []string `json:"services"`
	PSK      string   `json:"psk"`
}

func (s *Server) handleClient(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req clientReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}
		var res admin.AddUserResult
		res, err = admin.AddUser(cfg, req.Router, admin.UserOptions{
			Name: req.Name, ClientID: req.ClientID, Services: req.Services, PSK: req.PSK, Force: true,
		})
		cfg = res.Config
	case http.MethodDelete:
		q := r.URL.Query()
		cfg, err = admin.RemoveUserAccess(cfg, q.Get("router"), q.Get("name"))
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.saveAndRespond(w, cfg)
}

type userReq struct {
	Name     string `json:"name"`
	ClientID string `json:"client_id"`
	Rename   string `json:"rename"` // POST: when set, rename Name → Rename
}

// handleUser manages the top-level user entity: create, rename, delete. Per-router
// access grants live on /api/client.
func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req userReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}
		if req.Rename != "" {
			cfg, err = admin.RenameUser(cfg, req.Name, req.Rename)
		} else {
			cfg, err = admin.CreateUser(cfg, req.Name, req.ClientID)
		}
	case http.MethodDelete:
		cfg, err = admin.RemoveUser(cfg, r.URL.Query().Get("name"))
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.saveAndRespond(w, cfg)
}

type pskReq struct {
	User   string `json:"user"`
	Router string `json:"router"`
}

// handleUserPSK rotates the PSK for one (user, router) pair.
func (s *Server) handleUserPSK(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req pskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	cfg, err = admin.RotateUserPSK(cfg, req.User, req.Router)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.saveAndRespond(w, cfg)
}

func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rendered, err := admin.Render(cfg, r.URL.Query().Get("router"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(rendered))
}

// --- deploy ---

// deployReq drives a deploy action. Connection credentials are no longer part
// of it — they live on the router (config.Deploy), so the deploy screen only
// chooses an action and its modifiers.
type deployReq struct {
	Action string `json:"action"` // stream endpoint: status | apply | uninstall
	Router string `json:"router"`
	Force  bool   `json:"force"`
	DryRun bool   `json:"dry_run"`
}

func (s *Server) deployRequest(r *http.Request) (config.Config, deployReq, error) {
	var req deployReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return config.Config{}, req, err
	}
	cfg, err := config.LoadOrEmpty(s.configPath)
	return cfg, req, err
}

func (s *Server) handleDeployStatus(w http.ResponseWriter, r *http.Request) {
	cfg, req, err := s.deployRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := admin.Status(cfg, req.Router, admin.DeployOptions{})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDeployApply(w http.ResponseWriter, r *http.Request) {
	cfg, req, err := s.deployRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := admin.Apply(cfg, req.Router, admin.DeployOptions{}, req.Force, req.DryRun)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDeployUninstall(w http.ResponseWriter, r *http.Request) {
	cfg, req, err := s.deployRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := admin.Uninstall(cfg, req.Router, admin.DeployOptions{}, req.DryRun)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleDeployStream runs a deploy action and streams progress as newline-
// delimited JSON: {"type":"log","line":...} per SSH exchange as it happens, then
// a final {"type":"result",...} or {"type":"error",...}. The status is 200 up
// front (headers are already flushed), so failures arrive as an error event.
func (s *Server) handleDeployStream(w http.ResponseWriter, r *http.Request) {
	cfg, req, err := s.deployRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Flush live when possible (loopback serve); if the writer can't flush (e.g.
	// an embedded asset server), events still arrive, just batched at the end.
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	emit := func(v any) {
		_ = enc.Encode(v)
		if flusher != nil {
			flusher.Flush()
		}
	}
	emit(map[string]any{"type": "log", "line": "→ " + req.Action + " on " + req.Router + " …"})
	started := time.Now()
	action := req.Action
	if action == "" {
		action = "apply"
	}
	// Keep the router's own transcript out of the server log (it is long, and
	// it carries command output), but keep every line that says what the
	// operator was told went wrong.
	opts := admin.DeployOptions{OnLog: func(line string) {
		emit(map[string]any{"type": "log", "line": line})
		if strings.HasPrefix(line, "! ") {
			logf("deploy %s %s: %s", action, req.Router, strings.TrimPrefix(line, "! "))
		}
	}}

	var res any
	switch req.Action {
	case "status":
		res, err = admin.Status(cfg, req.Router, opts)
	case "uninstall":
		res, err = admin.Uninstall(cfg, req.Router, opts, req.DryRun)
	case "apply", "":
		res, err = admin.Apply(cfg, req.Router, opts, req.Force, req.DryRun)
	default:
		logf("deploy: unknown action %q", req.Action)
		emit(map[string]any{"type": "error", "msg": "unknown action: " + req.Action})
		return
	}
	if err != nil {
		logf("deploy %s %s: FAILED after %s: %v", action, req.Router, time.Since(started).Truncate(time.Millisecond), err)
		emit(map[string]any{"type": "error", "msg": err.Error()})
		return
	}
	logf("deploy %s %s: ok (%s)", action, req.Router, time.Since(started).Truncate(time.Millisecond))
	emit(map[string]any{"type": "result", "action": req.Action, "result": res})
}

// --- undo / redo ---

// save snapshots the current on-disk config into the undo history, clears the
// redo history, then writes the new config. Every config mutation goes through
// it so each step is undoable.
func (s *Server) save(cfg config.Config) error {
	s.mu.Lock()
	if prev, err := os.ReadFile(s.configPath); err == nil {
		s.undo = append(s.undo, prev)
		if len(s.undo) > maxUndo {
			s.undo = s.undo[len(s.undo)-maxUndo:]
		}
		s.redo = nil
	}
	s.mu.Unlock()
	return admin.SaveConfig(s.configPath, cfg)
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) { s.step(w, r, &s.undo, &s.redo) }
func (s *Server) handleRedo(w http.ResponseWriter, r *http.Request) { s.step(w, r, &s.redo, &s.undo) }

// step pops one snapshot from `from`, pushes the current state onto `to`, and
// restores the popped snapshot as the config. Undo and redo are symmetric.
func (s *Server) step(w http.ResponseWriter, r *http.Request, from, to *[][]byte) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.mu.Lock()
	if len(*from) == 0 {
		s.mu.Unlock()
		writeErr(w, http.StatusBadRequest, "nothing to do")
		return
	}
	cur, err := os.ReadFile(s.configPath)
	snap := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	if err == nil {
		*to = append(*to, cur)
	}
	s.mu.Unlock()

	if err := admin.WriteRaw(s.configPath, snap); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := config.LoadOrEmpty(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
}

// --- helpers ---

// writeConfigWarning is writeConfig plus a non-fatal warning the UI shows.
func (s *Server) writeConfigWarning(w http.ResponseWriter, cfg config.Config, warning string) {
	s.mu.Lock()
	canUndo, canRedo := len(s.undo) > 0, len(s.redo) > 0
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"path": s.configPath, "summary": admin.Summarize(cfg),
		"can_undo": canUndo, "can_redo": canRedo,
		"version": configVersion(s.configPath), "warning": warning,
	})
}

// saveAndRespond persists cfg and answers with the new state. It also compares
// the invite fingerprints before and after: an invite already in someone's
// hands dies the moment a PSK, a knock port, the router address or the bucket
// changes, and nothing else in the system would ever say so — a UDP knock
// cannot report delivery, so the user just sees "nothing opens".
func (s *Server) saveAndRespond(w http.ResponseWriter, cfg config.Config) {
	// The file still holds the pre-change state; reading it back beats keeping
	// a copy, because the handlers mutate maps the loaded config shares.
	before, _ := config.LoadOrEmpty(s.configPath)
	if err := s.save(cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfigStale(w, cfg, admin.InvalidatedInvites(before, cfg))
}

func (s *Server) writeConfigStale(w http.ResponseWriter, cfg config.Config, stale []string) {
	s.mu.Lock()
	canUndo, canRedo := len(s.undo) > 0, len(s.redo) > 0
	s.mu.Unlock()
	body := map[string]any{
		"path":     s.configPath,
		"summary":  admin.Summarize(cfg),
		"can_undo": canUndo,
		"can_redo": canRedo,
		"version":  configVersion(s.configPath),
	}
	if len(stale) > 0 {
		body["stale_invites"] = stale
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) writeConfig(w http.ResponseWriter, cfg config.Config) {
	s.mu.Lock()
	canUndo, canRedo := len(s.undo) > 0, len(s.redo) > 0
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     s.configPath,
		"summary":  admin.Summarize(cfg),
		"can_undo": canUndo,
		"can_redo": canRedo,
		// The page echoes this back on every mutation so a save against a
		// config someone else already changed is rejected instead of
		// silently overwriting their work.
		"version": configVersion(s.configPath),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
