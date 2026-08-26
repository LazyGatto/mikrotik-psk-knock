package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// Auth is the shared-password gate for the networked provision UI. There are no
// accounts: one password lets an admin in, exactly as the owner decided. That
// trade is deliberate — it costs per-person attribution and revocation, which
// docs/threat-model.md records.
//
// Local use (loopback `serve`, the Wails desktop app) stays unauthenticated:
// the config never leaves the machine there, and requiring a password would only
// add friction to the single-operator case.

const (
	sessionCookie = "mkpk_session"
	// Sessions live in memory, so a restart logs everyone out — acceptable, and
	// safer than persisting session material next to router credentials.
	sessionIdle     = 12 * time.Hour
	sessionAbsolute = 7 * 24 * time.Hour
	// Throttling: after this many consecutive failures the delay grows, capped
	// so a stuck admin is not locked out forever.
	throttleAfter   = 3
	throttleMaxWait = 30 * time.Second
	minPasswordLen  = 8
)

// argon2id parameters — interactive-login profile (64 MiB, 1 pass, 4 lanes).
const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// Credentials is the on-disk admin password record. It lives beside the config
// rather than inside it: mkpk.yaml gets copied, diffed and handed around, and
// authentication material has no business travelling with it.
type Credentials struct {
	V         int    `json:"v"`
	Algo      string `json:"algo"`
	Salt      string `json:"salt"`
	Hash      string `json:"hash"`
	Time      uint32 `json:"time"`
	Memory    uint32 `json:"memory"`
	Threads   uint8  `json:"threads"`
	UpdatedAt string `json:"updated_at"`
}

// CredentialsPath returns the admin credentials file for a given config path.
func CredentialsPath(configPath string) string {
	dir := filepath.Dir(configPath)
	return filepath.Join(dir, "mkpk-admin.json")
}

// HashPassword derives an argon2id record for a password.
func HashPassword(password string) (Credentials, error) {
	if len([]rune(password)) < minPasswordLen {
		return Credentials{}, fmt.Errorf("password must be at least %d characters", minPasswordLen)
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return Credentials{}, err
	}
	sum := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return Credentials{
		V: 1, Algo: "argon2id",
		Salt: base64.RawStdEncoding.EncodeToString(salt),
		Hash: base64.RawStdEncoding.EncodeToString(sum),
		Time: argonTime, Memory: argonMemory, Threads: argonThreads,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Verify reports whether the password matches the record.
func (c Credentials) Verify(password string) bool {
	salt, err := base64.RawStdEncoding.DecodeString(c.Salt)
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(c.Hash)
	if err != nil {
		return false
	}
	t, m, p := c.Time, c.Memory, c.Threads
	if t == 0 || m == 0 || p == 0 {
		t, m, p = argonTime, argonMemory, argonThreads
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// LoadCredentials reads the credentials file; ok is false when it is absent.
func LoadCredentials(path string) (Credentials, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Credentials{}, false, nil
	}
	if err != nil {
		return Credentials{}, false, err
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return Credentials{}, false, fmt.Errorf("admin credentials %s: %w", path, err)
	}
	if c.Hash == "" || c.Salt == "" {
		return Credentials{}, false, fmt.Errorf("admin credentials %s: incomplete record", path)
	}
	return c, true, nil
}

// SaveCredentials writes the record atomically with 0600 permissions.
func SaveCredentials(path string, c Credentials) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// session is one logged-in browser.
type session struct {
	csrf    string // per-session token injected into the page, required on /api
	created time.Time
	seen    time.Time
}

// Auth holds the password record and live sessions.
type Auth struct {
	path   string // credentials file
	secure bool   // mark cookies Secure (TLS terminated here or at the proxy)

	mu       sync.Mutex
	creds    Credentials
	sessions map[string]*session
	fails    int
	lastFail time.Time
}

// NewAuth builds the gate around an existing credentials file.
func NewAuth(path string, creds Credentials, secure bool) *Auth {
	return &Auth{path: path, creds: creds, secure: secure, sessions: map[string]*session{}}
}

// throttleWait returns how long a failed attempt should be delayed. The delay
// grows with consecutive failures so an exposed instance cannot be brute-forced,
// and decays once the attacker (or the admin) stops for a while.
func (a *Auth) throttleWait() time.Duration {
	if a.fails <= throttleAfter {
		return 0
	}
	if time.Since(a.lastFail) > 15*time.Minute {
		a.fails = 0
		return 0
	}
	wait := time.Duration(1<<uint(min(a.fails-throttleAfter, 6))) * time.Second
	if wait > throttleMaxWait {
		wait = throttleMaxWait
	}
	return wait
}

// Login checks the password and, on success, creates a session; it returns the
// session id and its CSRF token.
func (a *Auth) Login(password string) (id, csrf string, err error) {
	a.mu.Lock()
	wait := a.throttleWait()
	creds := a.creds
	a.mu.Unlock()

	if wait > 0 {
		time.Sleep(wait) // slow, deliberate: the caller is a login attempt
	}
	if !creds.Verify(password) {
		a.mu.Lock()
		a.fails++
		a.lastFail = time.Now()
		a.mu.Unlock()
		return "", "", fmt.Errorf("wrong password")
	}

	id, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	csrf, err = randomToken(24)
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	a.mu.Lock()
	a.fails = 0
	a.sessions[id] = &session{csrf: csrf, created: now, seen: now}
	a.mu.Unlock()
	return id, csrf, nil
}

// Session returns the live session for a request, refreshing its idle timer.
func (a *Auth) Session(r *http.Request) (*session, bool) {
	ck, err := r.Cookie(sessionCookie)
	if err != nil || ck.Value == "" {
		return nil, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[ck.Value]
	if !ok {
		return nil, false
	}
	now := time.Now()
	if now.Sub(s.seen) > sessionIdle || now.Sub(s.created) > sessionAbsolute {
		delete(a.sessions, ck.Value)
		return nil, false
	}
	s.seen = now
	return s, true
}

// Logout drops the request's session.
func (a *Auth) Logout(r *http.Request) {
	ck, err := r.Cookie(sessionCookie)
	if err != nil {
		return
	}
	a.mu.Lock()
	delete(a.sessions, ck.Value)
	a.mu.Unlock()
}

// ChangePassword replaces the stored password and drops every session except
// the caller's — a changed password must log the other browsers out.
func (a *Auth) ChangePassword(r *http.Request, current, next string) error {
	a.mu.Lock()
	creds := a.creds
	a.mu.Unlock()
	if !creds.Verify(current) {
		return fmt.Errorf("current password is wrong")
	}
	rec, err := HashPassword(next)
	if err != nil {
		return err
	}
	if err := SaveCredentials(a.path, rec); err != nil {
		return err
	}
	keep := ""
	if ck, err := r.Cookie(sessionCookie); err == nil {
		keep = ck.Value
	}
	a.mu.Lock()
	a.creds = rec
	for id := range a.sessions {
		if id != keep {
			delete(a.sessions, id)
		}
	}
	a.mu.Unlock()
	return nil
}

// SetCookie writes the session cookie for a fresh login.
func (a *Auth) SetCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/",
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode,
		MaxAge: int(sessionAbsolute / time.Second),
	})
}

// ClearCookie expires the session cookie.
func (a *Auth) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/",
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// BootstrapPassword sets the initial password from the environment when no
// credentials file exists yet. An existing file always wins: a stale env var in
// a compose file must not silently reset the admin password.
func BootstrapPassword(path, envPassword string) (Credentials, bool, error) {
	creds, ok, err := LoadCredentials(path)
	if err != nil {
		return Credentials{}, false, err
	}
	if ok {
		return creds, true, nil
	}
	if strings.TrimSpace(envPassword) == "" {
		return Credentials{}, false, nil
	}
	rec, err := HashPassword(envPassword)
	if err != nil {
		return Credentials{}, false, err
	}
	if err := SaveCredentials(path, rec); err != nil {
		return Credentials{}, false, err
	}
	return rec, true, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
