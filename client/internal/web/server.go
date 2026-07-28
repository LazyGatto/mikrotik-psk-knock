// Package web serves a local (127.0.0.1) admin UI over the internal/admin core:
// view and edit the config, render the .rsc and deploy over SSH. It is a thin
// HTTP frontend — all real work is in internal/admin. A per-session token gates
// the API so other-origin pages in the browser cannot drive it.
package web

import (
	"embed"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"mikrotik-psk-knock/client/internal/admin"
	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/version"
)

//go:embed assets/index.html assets/app.js assets/style.css assets/favicon-16.png assets/favicon-32.png assets/apple-touch-icon.png assets/logo-96.png assets/icon-512.png
var assetsFS embed.FS

const maxUndo = 100

// Server holds the config path, session token and the undo/redo history. History
// is per running session, in memory: raw config-file snapshots (the config is
// tiny). A single local operator uses it, so a mutex is enough for safety.
type Server struct {
	configPath string
	token      string
	mu         sync.Mutex
	undo       [][]byte // snapshots of the file before each mutation
	redo       [][]byte
}

// Handler builds the HTTP handler for the local admin UI.
func Handler(configPath, token string) http.Handler {
	s := &Server{configPath: configPath, token: token}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/app.js", s.static("assets/app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("/style.css", s.static("assets/style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/favicon.ico", s.static("assets/favicon-32.png", "image/png"))
	mux.HandleFunc("/favicon-16.png", s.static("assets/favicon-16.png", "image/png"))
	mux.HandleFunc("/favicon-32.png", s.static("assets/favicon-32.png", "image/png"))
	mux.HandleFunc("/apple-touch-icon.png", s.static("assets/apple-touch-icon.png", "image/png"))
	mux.HandleFunc("/logo-96.png", s.static("assets/logo-96.png", "image/png"))
	mux.HandleFunc("/icon-512.png", s.static("assets/icon-512.png", "image/png"))
	mux.HandleFunc("/api/config", s.auth(s.handleConfig))
	mux.HandleFunc("/api/secret", s.auth(s.handleSecret))
	mux.HandleFunc("/api/ports/suggest", s.auth(s.handlePortsSuggest))
	mux.HandleFunc("/api/router", s.auth(s.handleRouter))
	mux.HandleFunc("/api/router/info", s.auth(s.handleRouterInfo))
	mux.HandleFunc("/api/service", s.auth(s.handleService))
	mux.HandleFunc("/api/service/enable", s.auth(s.handleServiceEnable))
	mux.HandleFunc("/api/note", s.auth(s.handleNote))
	mux.HandleFunc("/api/client", s.auth(s.handleClient))
	mux.HandleFunc("/api/user", s.auth(s.handleUser))
	mux.HandleFunc("/api/user/psk", s.auth(s.handleUserPSK))
	mux.HandleFunc("/api/undo", s.auth(s.handleUndo))
	mux.HandleFunc("/api/redo", s.auth(s.handleRedo))
	mux.HandleFunc("/api/export", s.auth(s.handleExport))
	mux.HandleFunc("/api/render", s.auth(s.handleRender))
	mux.HandleFunc("/api/deploy/status", s.auth(s.handleDeployStatus))
	mux.HandleFunc("/api/deploy/apply", s.auth(s.handleDeployApply))
	mux.HandleFunc("/api/deploy/uninstall", s.auth(s.handleDeployUninstall))
	return loopbackOnly(mux)
}

// LogRequests wraps h to log each request (method, path, status, duration). API
// endpoints only — static asset noise is skipped.
func LogRequests(h http.Handler, l *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			l.Printf("%s %s%s -> %d (%s)", r.Method, r.URL.Path, querySuffix(r), rec.status, time.Since(start).Truncate(time.Millisecond))
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func querySuffix(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}

// loopbackOnly rejects requests whose Host is not localhost, which blocks
// DNS-rebinding attacks against the local server.
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
	data, err := assetsFS.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := strings.Replace(string(data), "__MKPK_TOKEN__", s.token, 1)
	page = strings.Replace(page, "__MKPK_VERSION__", version.String(), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
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

// auth gates API calls with the session token, so only our own page can call them.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MKPK-Token") != s.token {
			writeErr(w, http.StatusForbidden, "invalid session token")
			return
		}
		next(w, r)
	}
}

// --- config ---

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
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
	cfg, err := config.Load(s.configPath)
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
	Name          string        `json:"name"`
	Rename        string        `json:"rename"` // on edit: new name for the router
	Address       string        `json:"address"`
	DeployAddress string        `json:"deploy_address"` // optional SSH override
	Port          int           `json:"port"`
	User          string        `json:"user"`
	KeyPath       string        `json:"key_path"`
	KeyPass       string        `json:"key_pass"`
	UseAgent      bool          `json:"use_agent"`
	Password      string        `json:"password"`
	Notify        config.Notify `json:"notify"`
}

// handleRouterInfo returns a live health snapshot (device info + install state)
// for the router named in ?router=. The UI polls it periodically.
func (s *Server) handleRouterInfo(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
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
	cfg, err := config.Load(s.configPath)
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
		cfg, err = admin.SetRouter(cfg, admin.RouterOptions{Name: target, Address: req.Address, Deploy: dep, Notify: notify})
	case http.MethodDelete:
		cfg, err = admin.RemoveRouter(cfg, r.URL.Query().Get("name"))
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.save(cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
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
	cfg, err := config.Load(s.configPath)
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
	if err := s.save(cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
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
	cfg, err := config.Load(s.configPath)
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
	if err := s.save(cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
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
	Router      string        `json:"router"`
	Name        string        `json:"name"`
	Rename      string        `json:"rename"` // on edit: new name for the service
	ServiceName string        `json:"service_name"`
	Disabled    bool          `json:"disabled"`
	Stage1Port  int           `json:"stage1_port"`
	Stage2Port  int           `json:"stage2_port"`
	TokenPort   int           `json:"token_port"`
	AllowedList string        `json:"allowed_list"`
	Target      config.Target `json:"target"`
	Force       bool          `json:"force"`
}

func (s *Server) handleService(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
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
			AllowedList: req.AllowedList, Target: req.Target, Force: true,
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
	if err := s.save(cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
}

type clientReq struct {
	Router   string   `json:"router"`
	Name     string   `json:"name"`
	ClientID string   `json:"client_id"`
	Services []string `json:"services"`
	PSK      string   `json:"psk"`
}

func (s *Server) handleClient(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
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
	if err := s.save(cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
}

type userReq struct {
	Name     string `json:"name"`
	ClientID string `json:"client_id"`
	Rename   string `json:"rename"` // POST: when set, rename Name → Rename
}

// handleUser manages the top-level user entity: create, rename, delete. Per-router
// access grants live on /api/client.
func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
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
	if err := s.save(cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
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
	cfg, err := config.Load(s.configPath)
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
	if err := s.save(cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
}

func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
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
	Router string `json:"router"`
	Force  bool   `json:"force"`
	DryRun bool   `json:"dry_run"`
}

func (s *Server) deployRequest(r *http.Request) (config.Config, deployReq, error) {
	var req deployReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return config.Config{}, req, err
	}
	cfg, err := config.Load(s.configPath)
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
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeConfig(w, cfg)
}

// --- helpers ---

func (s *Server) writeConfig(w http.ResponseWriter, cfg config.Config) {
	s.mu.Lock()
	canUndo, canRedo := len(s.undo) > 0, len(s.redo) > 0
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     s.configPath,
		"summary":  admin.Summarize(cfg),
		"can_undo": canUndo,
		"can_redo": canRedo,
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
