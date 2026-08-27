package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"mikrotik-psk-knock/client/internal/release"
)

// The update check itself lives in internal/release — the Windows client asks
// the same question, and one implementation is one place to fix.

// openURLPrefix limits /api/open targets to our own release pages — the
// endpoint must not become a generic URL launcher.
const openURLPrefix = release.PageURLPrefix

// handleUpdate reports whether a newer release exists on the public mirror.
// The result is cached per process so the UI can ask freely.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	info, err := s.updates.Get()
	if err != nil {
		writeErr(w, http.StatusBadGateway, "update check: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleOpen opens one of our release pages in the system browser. Desktop
// only — the browser UI opens links itself.
func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.hooks.OpenURL == nil {
		writeErr(w, http.StatusBadRequest, "no browser hook in this mode")
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if !strings.HasPrefix(req.URL, openURLPrefix) {
		writeErr(w, http.StatusBadRequest, "url outside the release pages")
		return
	}
	if err := s.hooks.OpenURL(req.URL); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
