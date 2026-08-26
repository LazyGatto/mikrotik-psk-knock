package desktopui

import (
	"testing"
	"time"
)

func TestParseGoDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"55m":   55 * time.Minute,
		"1h30m": 90 * time.Minute,
		"45s":   45 * time.Second,
		"2d":    48 * time.Hour,
		"1d2h":  26 * time.Hour,
		"1w":    7 * 24 * time.Hour,
	}
	for in, want := range cases {
		got, err := parseGoDuration(in)
		if err != nil || got != want {
			t.Errorf("parseGoDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "5x"} {
		if _, err := parseGoDuration(bad); err == nil {
			t.Errorf("parseGoDuration(%q) succeeded, want error", bad)
		}
	}
}

func TestStatusRegistry(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(store, "tok", KnockTimings{})
	if _, err := store.Add("reg", testBlobRaw(t, 443)); err != nil {
		t.Fatal(err)
	}

	changed := 0
	srv.SetOnChange(func() { changed++ })

	states, err := srv.Status()
	if err != nil || len(states) != 1 {
		t.Fatalf("states = %v, %v", states, err)
	}
	if states[0].Open() {
		t.Fatalf("fresh service reported open")
	}

	srv.state.markOpen(states[0].InviteID, states[0].Router, states[0].Service, time.Now().Add(time.Minute))
	if changed != 1 {
		t.Fatalf("onChange fired %d times, want 1", changed)
	}
	states, _ = srv.Status()
	if !states[0].Open() {
		t.Fatalf("service not open after markOpen: %+v", states[0])
	}

	srv.state.markOpen(states[0].InviteID, states[0].Router, states[0].Service, time.Now().Add(-time.Second))
	states, _ = srv.Status()
	if states[0].Open() {
		t.Fatalf("expired entry reported open")
	}
}
