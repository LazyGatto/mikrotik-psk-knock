// Package release answers one question for every mkpk frontend: is there a
// newer version than the one running? The admin console and the Windows client
// both ask it, so the feed URL, the semver comparison and the "a dev build
// never nags" rule live here once rather than drifting apart in two copies.
package release

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mikrotik-psk-knock/client/internal/version"
)

// FeedURL is the public GitHub mirror's latest release (the GitLab host stays
// out of public-facing code). Var so tests can point it at a stub.
var FeedURL = "https://api.github.com/repos/LazyGatto/mikrotik-psk-knock/releases/latest"

// PageURLPrefix bounds every link a frontend may hand to the system browser:
// these endpoints open our own release pages, never an arbitrary URL.
const PageURLPrefix = "https://github.com/LazyGatto/mikrotik-psk-knock/"

// Info is what a frontend needs to decide whether to say anything at all.
type Info struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Newer   bool   `json:"newer"`
	URL     string `json:"url"`
}

// Check asks the mirror once, without caching. Callers that ask on a timer
// should use Cache.
func Check() (Info, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest(http.MethodGet, FeedURL, nil)
	if err != nil {
		return Info{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := client.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Info{}, fmt.Errorf("releases feed: HTTP %d", res.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rel); err != nil {
		return Info{}, err
	}
	cur := version.String()
	info := Info{Current: cur, Latest: rel.TagName, URL: rel.HTMLURL}
	// A dev build ("dev", or an unparseable string) never nags about updates.
	if curV, ok := ParseSemver(cur); ok {
		if latestV, ok := ParseSemver(rel.TagName); ok {
			info.Newer = less(curV, latestV)
		}
	}
	return info, nil
}

// Cache holds one Check result for TTL, so a UI may ask as often as it likes
// without hammering the feed (GitHub rate-limits unauthenticated callers).
type Cache struct {
	TTL time.Duration // zero → 6h

	mu      sync.Mutex
	info    Info
	fetched time.Time
}

// Get returns the cached answer, refreshing it when stale.
func (c *Cache) Get() (Info, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := c.TTL
	if ttl == 0 {
		ttl = 6 * time.Hour
	}
	if c.info.Latest != "" && time.Since(c.fetched) < ttl {
		return c.info, nil
	}
	info, err := Check()
	if err != nil {
		return Info{}, err
	}
	c.info, c.fetched = info, time.Now()
	return info, nil
}

// ParseSemver extracts the leading X.Y.Z from "v0.6.0", "0.6.0" or
// "v0.6.0-3-gabc" (git describe of a build between tags).
func ParseSemver(s string) ([3]int, bool) {
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
