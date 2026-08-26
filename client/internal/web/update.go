package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mikrotik-psk-knock/client/internal/version"
)

// The public releases live on the GitHub mirror (the GitLab host stays out of
// public-facing code). Var so tests can point it at a stub.
var updateFeedURL = "https://api.github.com/repos/LazyGatto/mikrotik-psk-knock/releases/latest"

// openURLPrefix limits /api/open targets to our own release pages — the
// endpoint must not become a generic URL launcher.
const openURLPrefix = "https://github.com/LazyGatto/mikrotik-psk-knock/"

const updateCacheTTL = 6 * time.Hour

type updateInfo struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Newer   bool   `json:"newer"`
	URL     string `json:"url"`
}

// handleUpdate reports whether a newer release exists on the public mirror.
// The result is cached per process so the UI can ask freely.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	s.updMu.Lock()
	defer s.updMu.Unlock()
	if time.Since(s.updFetched) > updateCacheTTL || s.updCache.Latest == "" {
		info, err := fetchUpdateInfo()
		if err != nil {
			writeErr(w, http.StatusBadGateway, "update check: "+err.Error())
			return
		}
		s.updCache = info
		s.updFetched = time.Now()
	}
	writeJSON(w, http.StatusOK, s.updCache)
}

func fetchUpdateInfo() (updateInfo, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest(http.MethodGet, updateFeedURL, nil)
	if err != nil {
		return updateInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := client.Do(req)
	if err != nil {
		return updateInfo{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return updateInfo{}, fmt.Errorf("releases feed: HTTP %d", res.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rel); err != nil {
		return updateInfo{}, err
	}
	cur := version.String()
	info := updateInfo{Current: cur, Latest: rel.TagName, URL: rel.HTMLURL}
	// A dev build ("dev", or an unparseable string) never nags about updates.
	if curV, ok := parseSemver(cur); ok {
		if latestV, ok := parseSemver(rel.TagName); ok {
			info.Newer = less(curV, latestV)
		}
	}
	return info, nil
}

// parseSemver extracts the leading X.Y.Z from "v0.6.0", "0.6.0" or
// "v0.6.0-3-gabc" (git describe of a build between tags).
func parseSemver(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var v [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		v[i] = n
	}
	return v, true
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
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
