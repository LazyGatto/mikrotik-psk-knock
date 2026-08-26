// Package desktopui is the recipient-side desktop client UI: a small embedded
// HTTP handler (mounted as the Wails asset server by cmd/mkpk-desktop) plus a
// file store for imported .mkpk invites. It drives the same runtime packages as
// the mkpk CLI (invite, token, knock, servicecheck) — no protocol code of its
// own.
package desktopui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mikrotik-psk-knock/client/internal/invite"
)

// Store keeps imported invites as files (one blob per file, stored exactly as
// imported) plus a small settings.json, under dir — %APPDATA%\mkpk on Windows,
// ~/Library/Application Support/mkpk on macOS (os.UserConfigDir()/mkpk).
type Store struct {
	dir string
}

// StoredInvite is one imported invite: its file-derived id, display name and
// the decoded blob.
type StoredInvite struct {
	ID     string
	Name   string
	Invite invite.Blob
}

// Settings is the persisted UI state.
type Settings struct {
	Language string `json:"language,omitempty"` // "en" (default) | "ru"
	Theme    string `json:"theme,omitempty"`    // "dark" (default) | "light"
	// Launch holds per-service commands the USER typed, keyed by
	// "inviteID/router/service". Nothing here ever comes from an invite: an
	// invite is unsigned, so a command carried in one would be arbitrary code
	// execution on import. The local file is 0600 like the invites.
	Launch map[string]string `json:"launch,omitempty"`
}

// DefaultDir returns the per-user store directory.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("desktopui: user config dir: %w", err)
	}
	return filepath.Join(base, "mkpk"), nil
}

// NewStore opens (creating if needed) the store at dir.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "invites"), 0o700); err != nil {
		return nil, fmt.Errorf("desktopui: create store: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Add validates and saves a new invite blob under the given display name.
// The blob is stored verbatim, so a re-export equals the original invite.
func (s *Store) Add(name, blob string) (StoredInvite, error) {
	blob = strings.TrimSpace(blob)
	b, err := invite.Decode(blob)
	if err != nil {
		return StoredInvite{}, err
	}
	id := safeID(name)
	if id == "" {
		id = safeID(b.ClientID)
	}
	if id == "" {
		id = "invite"
	}
	// Avoid clobbering a different invite with the same name.
	path := s.invitePath(id)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		id = fmt.Sprintf("%s-%d", safeID(name), i)
		path = s.invitePath(id)
	}
	if err := os.WriteFile(path, []byte(blob+"\n"), 0o600); err != nil {
		return StoredInvite{}, fmt.Errorf("desktopui: save invite: %w", err)
	}
	return StoredInvite{ID: id, Name: id, Invite: b}, nil
}

// List returns all stored invites sorted by id. A file that no longer decodes
// is reported as an error — fail loudly, do not silently hide a broken invite.
func (s *Store) List() ([]StoredInvite, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "invites"))
	if err != nil {
		return nil, fmt.Errorf("desktopui: list invites: %w", err)
	}
	var out []StoredInvite
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mkpk") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".mkpk")
		data, err := os.ReadFile(s.invitePath(id))
		if err != nil {
			return nil, fmt.Errorf("desktopui: read invite %s: %w", id, err)
		}
		b, err := invite.Decode(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, fmt.Errorf("desktopui: invite %s: %w", id, err)
		}
		out = append(out, StoredInvite{ID: id, Name: id, Invite: b})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get returns one stored invite by id.
func (s *Store) Get(id string) (StoredInvite, error) {
	list, err := s.List()
	if err != nil {
		return StoredInvite{}, err
	}
	for _, inv := range list {
		if inv.ID == id {
			return inv, nil
		}
	}
	return StoredInvite{}, fmt.Errorf("desktopui: invite %q not found", id)
}

// Remove deletes a stored invite.
func (s *Store) Remove(id string) error {
	if id != safeID(id) || id == "" {
		return fmt.Errorf("desktopui: bad invite id %q", id)
	}
	if err := os.Remove(s.invitePath(id)); err != nil {
		return fmt.Errorf("desktopui: remove invite: %w", err)
	}
	return nil
}

// Settings loads the persisted settings (zero value when absent).
func (s *Store) Settings() Settings {
	var st Settings
	data, err := os.ReadFile(filepath.Join(s.dir, "settings.json"))
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st) // broken settings file → defaults
	return st
}

// SaveSettings persists the settings.
func (s *Store) SaveSettings(st Settings) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.dir, "settings.json"), data, 0o600); err != nil {
		return fmt.Errorf("desktopui: save settings: %w", err)
	}
	return nil
}

func (s *Store) invitePath(id string) string {
	return filepath.Join(s.dir, "invites", id+".mkpk")
}

// safeID reduces a display name to a filesystem-safe id.
func safeID(name string) string {
	name = strings.TrimSuffix(filepath.Base(name), ".mkpk")
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			sb.WriteRune(r)
		case r == ' ', r == '.':
			sb.WriteRune('-')
		}
	}
	return strings.Trim(sb.String(), "-")
}
