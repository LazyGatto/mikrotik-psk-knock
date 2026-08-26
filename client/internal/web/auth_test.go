package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPassword = "correct-horse-battery"

func newAuthServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mkpk.yaml")
	// A fresh install has no config file yet — that is what LoadOrEmpty treats
	// as an empty config; an existing file is parsed and validated instead.
	credPath := CredentialsPath(cfgPath)
	rec, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveCredentials(credPath, rec); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(AuthHandler(cfgPath, NewAuth(credPath, rec, false)))
	t.Cleanup(ts.Close)
	return ts, cfgPath
}

// client that keeps cookies but never follows redirects, so we can assert them.
func newClient(ts *httptest.Server) *http.Client {
	jar := &cookieJar{}
	return &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func login(t *testing.T, ts *httptest.Server, c *http.Client, password string) *http.Response {
	t.Helper()
	res, err := c.PostForm(ts.URL+"/login", map[string][]string{"password": {password}})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return res
}

func TestAPIRefusesWithoutSession(t *testing.T) {
	ts, _ := newAuthServer(t)
	res, err := http.Get(ts.URL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}

func TestIndexServesLoginPageWhenSignedOut(t *testing.T) {
	ts, _ := newAuthServer(t)
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body := readAll(t, res)
	if !strings.Contains(body, "/login") {
		t.Fatalf("index did not serve the login form: %.120s", body)
	}
	// The SPA's session token must not be handed to an anonymous visitor.
	if strings.Contains(body, "MKPK_TOKEN") {
		t.Fatal("login page leaked the app token")
	}
}

func TestWrongPasswordIsRejected(t *testing.T) {
	ts, _ := newAuthServer(t)
	c := newClient(ts)
	res := login(t, ts, c, "not-the-password")
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
	if len(res.Cookies()) > 0 {
		t.Fatalf("a failed login set cookies: %v", res.Cookies())
	}
}

func TestLoginThenUseAPI(t *testing.T) {
	ts, _ := newAuthServer(t)
	c := newClient(ts)

	res := login(t, ts, c, testPassword)
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want 303", res.StatusCode)
	}
	var sessionCk *http.Cookie
	for _, ck := range res.Cookies() {
		if ck.Name == sessionCookie {
			sessionCk = ck
		}
	}
	if sessionCk == nil {
		t.Fatal("no session cookie after login")
	}
	if !sessionCk.HttpOnly || sessionCk.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie is not HttpOnly+SameSite=Strict: %+v", sessionCk)
	}

	// The page carries the session's CSRF token; the API needs it.
	page := getWithCookie(t, ts, "/", sessionCk)
	token := extractToken(t, page)
	if token == "" {
		t.Fatal("page did not embed a session token")
	}

	// Without the header the request is refused even with a valid cookie.
	req, _ := http.NewRequest("GET", ts.URL+"/api/config", nil)
	req.AddCookie(sessionCk)
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != http.StatusForbidden {
		t.Fatalf("status without CSRF token = %d, want 403", res2.StatusCode)
	}

	// With both, it works.
	req, _ = http.NewRequest("GET", ts.URL+"/api/config", nil)
	req.AddCookie(sessionCk)
	req.Header.Set("X-MKPK-Token", token)
	res3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res3.StatusCode)
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(res3.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Path == "" {
		t.Fatal("config response is empty")
	}
}

// TestConfigVersionTracksTheFile backs the lost-update guard: the version must
// change whenever the file does, and be empty when there is no file yet.
func TestConfigVersionTracksTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mkpk.yaml")
	if v := configVersion(path); v != "" {
		t.Fatalf("missing file version = %q, want empty", v)
	}
	if err := os.WriteFile(path, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := configVersion(path)
	if first == "" {
		t.Fatal("existing file has no version")
	}
	if configVersion(path) != first {
		t.Fatal("version is not stable for unchanged content")
	}
	if err := os.WriteFile(path, []byte("a: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if configVersion(path) == first {
		t.Fatal("version did not change after an edit")
	}
}

// TestStaleConfigVersionIsRejected is the shared-instance guard: a save aimed at
// a config another admin already changed must fail loudly, not overwrite.
func TestStaleConfigVersionIsRejected(t *testing.T) {
	ts, cfgPath := newAuthServer(t)
	c := newClient(ts)
	res := login(t, ts, c, testPassword)
	res.Body.Close()
	var ck *http.Cookie
	for _, x := range res.Cookies() {
		if x.Name == sessionCookie {
			ck = x
		}
	}
	token := extractToken(t, getWithCookie(t, ts, "/", ck))

	// Someone else edits the config on disk.
	if err := os.WriteFile(cfgPath, []byte("# edited elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest("POST", ts.URL+"/api/user", strings.NewReader(`{"name":"laptop"}`))
	req.AddCookie(ck)
	req.Header.Set("X-MKPK-Token", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MKPK-Config-Version", "stale-version-from-an-old-page")
	out, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Body.Close()
	if out.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", out.StatusCode)
	}
}

func TestPasswordRoundTripAndBootstrap(t *testing.T) {
	rec, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Verify(testPassword) {
		t.Fatal("password does not verify against its own record")
	}
	if rec.Verify(testPassword + "x") {
		t.Fatal("a wrong password verified")
	}
	if strings.Contains(rec.Hash, testPassword) || strings.Contains(rec.Salt, testPassword) {
		t.Fatal("the record embeds the plaintext password")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("a too-short password was accepted")
	}

	// Bootstrap writes the file once; an existing file always wins so a stale
	// env var cannot silently reset the admin password.
	dir := t.TempDir()
	path := filepath.Join(dir, "mkpk-admin.json")
	if _, ok, err := BootstrapPassword(path, "first-password-here"); err != nil || !ok {
		t.Fatalf("bootstrap: ok=%v err=%v", ok, err)
	}
	got, ok, err := BootstrapPassword(path, "second-password-here")
	if err != nil || !ok {
		t.Fatalf("second bootstrap: ok=%v err=%v", ok, err)
	}
	if !got.Verify("first-password-here") {
		t.Fatal("env password overwrote an existing credentials file")
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("credentials file mode = %v (err %v), want 0600", fi.Mode().Perm(), err)
	}
}

// --- helpers -----------------------------------------------------------------

func readAll(t *testing.T, res *http.Response) string {
	t.Helper()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := res.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

func getWithCookie(t *testing.T, ts *httptest.Server, path string, ck *http.Cookie) string {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	if ck != nil {
		req.AddCookie(ck)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	return readAll(t, res)
}

func extractToken(t *testing.T, page string) string {
	t.Helper()
	const marker = `window.MKPK_TOKEN = "`
	i := strings.Index(page, marker)
	if i < 0 {
		return ""
	}
	rest := page[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// cookieJar is a minimal in-memory jar (net/http/cookiejar needs a public
// suffix list for 127.0.0.1 to behave predictably in tests).
type cookieJar struct{ cookies []*http.Cookie }

func (j *cookieJar) SetCookies(_ *neturl.URL, cookies []*http.Cookie) { j.cookies = cookies }
func (j *cookieJar) Cookies(_ *neturl.URL) []*http.Cookie             { return j.cookies }
