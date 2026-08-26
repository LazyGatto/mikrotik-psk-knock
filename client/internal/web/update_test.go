package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mikrotik-psk-knock/client/internal/version"
)

func TestParseSemver(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
		ok   bool
	}{
		{"v0.6.0", [3]int{0, 6, 0}, true},
		{"0.6.0", [3]int{0, 6, 0}, true},
		{"v0.5.0-3-gfea4003", [3]int{0, 5, 0}, true},
		{"dev", [3]int{}, false},
		{"", [3]int{}, false},
	}
	for _, c := range cases {
		got, ok := parseSemver(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("parseSemver(%q) = %v,%v; want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestUpdateCheck(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://github.com/LazyGatto/mikrotik-psk-knock/releases/tag/v9.9.9"}`))
	}))
	defer feed.Close()
	oldURL, oldVer := updateFeedURL, version.Version
	updateFeedURL, version.Version = feed.URL, "v0.5.0"
	defer func() { updateFeedURL, version.Version = oldURL, oldVer }()

	info, err := fetchUpdateInfo()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !info.Newer || info.Latest != "v9.9.9" {
		t.Fatalf("info = %+v, want newer v9.9.9", info)
	}

	// dev builds never nag
	version.Version = "dev"
	info, err = fetchUpdateInfo()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if info.Newer {
		t.Fatalf("dev build reported newer: %+v", info)
	}
}
