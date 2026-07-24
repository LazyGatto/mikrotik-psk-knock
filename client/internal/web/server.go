// Package web serves a local (127.0.0.1) admin UI over the internal/admin core:
// view and edit the config, render the .rsc and deploy over SSH. It is a thin
// HTTP frontend — all real work is in internal/admin. A per-session token gates
// the API so other-origin pages in the browser cannot drive it.
package web

import (
	"embed"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"mikrotik-psk-knock/client/internal/admin"
	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/deploy"
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
	mux.HandleFunc("/api/service", s.auth(s.handleService))
	mux.HandleFunc("/api/client", s.auth(s.handleClient))
	mux.HandleFunc("/api/render", s.auth(s.handleRender))
	mux.HandleFunc("/api/deploy/status", s.auth(s.handleDeployStatus))
	mux.HandleFunc("/api/deploy/apply", s.auth(s.handleDeployApply))
	mux.HandleFunc("/api/deploy/uninstall", s.auth(s.handleDeployUninstall))
	return loopbackOnly(mux)
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

type serviceReq struct {
	Name        string        `json:"name"`
	ServiceName string        `json:"service_name"`
	Stage1Port  int           `json:"stage1_port"`
	Stage2Port  int           `json:"stage2_port"`
	TokenPort   int           `json:"token_port"`
	AllowedList string        `json:"allowed_list"`
	NAT         config.NAT    `json:"nat"`
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
		cfg, err = admin.AddService(cfg, admin.ServiceOptions{
			Name: req.Name, ServiceName: req.ServiceName,
			Stage1Port: req.Stage1Port, Stage2Port: req.Stage2Port, TokenPort: req.TokenPort,
			AllowedList: req.AllowedList, NAT: req.NAT, Notify: req.Notify, Force: true,
		})
	case http.MethodDelete:
		cfg, err = admin.RemoveService(cfg, r.URL.Query().Get("name"))
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
	Name     string `json:"name"`
	ClientID string `json:"client_id"`
	Service  string `json:"service"`
	PSK      string `json:"psk"`
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
		res, err = admin.AddClient(cfg, admin.ClientOptions{
			Name: req.Name, ClientID: req.ClientID, Service: req.Service, PSK: req.PSK, Force: true,
		})
		cfg = res.Config
	case http.MethodDelete:
		cfg, err = admin.RemoveClient(cfg, r.URL.Query().Get("name"))
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
	rendered, err := admin.Render(cfg, r.URL.Query().Get("client"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(rendered))
}

// --- deploy ---

type deployReq struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	KeyPath  string `json:"key_path"`
	KeyPass  string `json:"key_pass"`
	UseAgent bool   `json:"use_agent"`
	Password string `json:"password"`
	Force    bool   `json:"force"`
	DryRun   bool   `json:"dry_run"`
}

func (r deployReq) options() admin.DeployOptions {
	return admin.DeployOptions{
		Address: r.Address,
		Port:    r.Port,
		Auth: deploy.Auth{
			User: r.User, KeyPath: r.KeyPath, KeyPass: r.KeyPass,
			UseAgent: r.UseAgent, Password: r.Password,
		},
	}
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
	res, err := admin.Status(cfg, req.options())
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
	res, err := admin.Apply(cfg, req.options(), req.Force, req.DryRun)
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
	addr, applied, err := admin.Uninstall(cfg, req.options(), req.DryRun)
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
		"hash":    cfg.Hash(),
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
