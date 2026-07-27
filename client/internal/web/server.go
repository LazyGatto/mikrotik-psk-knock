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
	"strconv"
	"strings"
	"time"

	"mikrotik-psk-knock/client/internal/admin"
	"mikrotik-psk-knock/client/internal/config"
)

//go:embed assets/index.html assets/app.js assets/style.css
var assetsFS embed.FS

// Server holds the config path and session token.
type Server struct {
	configPath string
	token      string
}

// Handler builds the HTTP handler for the local admin UI.
func Handler(configPath, token string) http.Handler {
	s := &Server{configPath: configPath, token: token}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/app.js", s.static("assets/app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("/style.css", s.static("assets/style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/api/config", s.auth(s.handleConfig))
	mux.HandleFunc("/api/secret", s.auth(s.handleSecret))
	mux.HandleFunc("/api/ports/suggest", s.auth(s.handlePortsSuggest))
	mux.HandleFunc("/api/router", s.auth(s.handleRouter))
	mux.HandleFunc("/api/service", s.auth(s.handleService))
	mux.HandleFunc("/api/service/enable", s.auth(s.handleServiceEnable))
	mux.HandleFunc("/api/client", s.auth(s.handleClient))
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
	writeConfig(w, s.configPath, cfg)
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
	Name     string `json:"name"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	KeyPath  string `json:"key_path"`
	KeyPass  string `json:"key_pass"`
	UseAgent bool   `json:"use_agent"`
	Password string `json:"password"`
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
			Port: req.Port, User: req.User, KeyPath: req.KeyPath,
			KeyPass: req.KeyPass, UseAgent: req.UseAgent, Password: req.Password,
		}
		// Secrets are never sent to the browser, so a blank secret on edit means
		// "keep the stored one" rather than "clear it".
		if existing, ok := cfg.Routers[req.Name]; ok {
			if dep.Password == "" {
				dep.Password = existing.Deploy.Password
			}
			if dep.KeyPass == "" {
				dep.KeyPass = existing.Deploy.KeyPass
			}
		}
		cfg, err = admin.SetRouter(cfg, admin.RouterOptions{Name: req.Name, Address: req.Address, Deploy: dep})
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
	if err := admin.SaveConfig(s.configPath, cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeConfig(w, s.configPath, cfg)
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
	if err := admin.SaveConfig(s.configPath, cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeConfig(w, s.configPath, cfg)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	blob, err := admin.ExportUser(cfg, q.Get("router"), q.Get("user"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"blob": blob})
}

type serviceReq struct {
	Router      string        `json:"router"`
	Name        string        `json:"name"`
	ServiceName string        `json:"service_name"`
	Disabled    bool          `json:"disabled"`
	Stage1Port  int           `json:"stage1_port"`
	Stage2Port  int           `json:"stage2_port"`
	TokenPort   int           `json:"token_port"`
	AllowedList string        `json:"allowed_list"`
	Target      config.Target `json:"target"`
	Notify      config.Notify `json:"notify"`
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
		cfg, err = admin.AddService(cfg, req.Router, admin.ServiceOptions{
			Name: req.Name, ServiceName: req.ServiceName, Disabled: req.Disabled,
			Stage1Port: req.Stage1Port, Stage2Port: req.Stage2Port, TokenPort: req.TokenPort,
			AllowedList: req.AllowedList, Target: req.Target, Notify: req.Notify, Force: true,
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
	if err := admin.SaveConfig(s.configPath, cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeConfig(w, s.configPath, cfg)
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
		var res admin.AddClientResult
		res, err = admin.AddClient(cfg, req.Router, admin.ClientOptions{
			Name: req.Name, ClientID: req.ClientID, Services: req.Services, PSK: req.PSK, Force: true,
		})
		cfg = res.Config
	case http.MethodDelete:
		q := r.URL.Query()
		cfg, err = admin.RemoveClient(cfg, q.Get("router"), q.Get("name"))
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := admin.SaveConfig(s.configPath, cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeConfig(w, s.configPath, cfg)
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
	addr, applied, err := admin.Uninstall(cfg, req.Router, admin.DeployOptions{}, req.DryRun)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"router": addr, "applied": applied})
}

// --- helpers ---

func writeConfig(w http.ResponseWriter, path string, cfg config.Config) {
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    path,
		"summary": admin.Summarize(cfg),
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
