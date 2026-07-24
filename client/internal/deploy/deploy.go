// Package deploy provisions the mkpk RouterOS layer over SSH: it detects whether
// the functionality is installed and up to date, uploads and imports a rendered
// .rsc, verifies the result, and can uninstall. SSH is only a provisioning
// channel; the runtime port-knocking path stays entirely client-side.
package deploy

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	remoteFileName = "mkpk-deploy.rsc"
	metaScript     = "mkpk-tt-meta"
)

// Auth describes how to authenticate to the router. Key auth is primary; password
// is a fallback.
type Auth struct {
	User     string
	KeyPath  string // private key file (primary)
	KeyPass  string // optional passphrase for the key
	UseAgent bool   // also try ssh-agent
	Password string // fallback
}

// Client is a connected SSH session to a router.
type Client struct {
	conn *ssh.Client
}

// State is the detected mkpk install state on a router.
type State struct {
	Installed bool
	Hash      string // config hash stamped in mkpk-tt-meta, empty if not installed
}

// Connect dials the router and authenticates. Host keys use trust-on-first-use:
// unknown hosts are appended to known_hosts, changed keys are rejected.
func Connect(address string, port int, auth Auth) (*Client, error) {
	methods, err := authMethods(auth)
	if err != nil {
		return nil, err
	}
	hostKey, err := tofuHostKeyCallback()
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            auth.User,
		Auth:            methods,
		HostKeyCallback: hostKey,
		Timeout:         15 * time.Second,
	}
	addr := net.JoinHostPort(address, fmt.Sprint(port))
	conn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return &Client{conn: conn}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// Run executes a RouterOS command and returns its combined output.
func (c *Client) Run(cmd string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

// Detect reports whether mkpk is installed and, if so, its stamped config hash.
func (c *Client) Detect() (State, error) {
	out, err := c.Run(`:if ([:len [/system script find where name="` + metaScript + `"]] > 0) do={ :put [/system script get [find where name="` + metaScript + `"] source] } else={ :put "NOT_INSTALLED" }`)
	if err != nil {
		return State{}, fmt.Errorf("detect: %w (%s)", err, strings.TrimSpace(out))
	}
	if strings.Contains(out, "NOT_INSTALLED") {
		return State{Installed: false}, nil
	}
	return State{Installed: true, Hash: parseMetaHash(out)}, nil
}

// Deploy uploads the rendered .rsc, imports it, removes the transient file and
// verifies the result.
func (c *Client) Deploy(rsc []byte) error {
	if err := c.upload(remoteFileName, rsc); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	out, err := c.Run("/import file-name=" + remoteFileName)
	if err != nil {
		return fmt.Errorf("import: %w (%s)", err, strings.TrimSpace(out))
	}
	if !strings.Contains(out, "loaded and executed successfully") {
		return fmt.Errorf("import did not report success: %s", strings.TrimSpace(out))
	}
	// Best-effort remove of the transient file (secrets live in the config anyway).
	_, _ = c.Run(`/file remove [find where name="` + remoteFileName + `"]`)
	return c.Verify()
}

// Verify waits for the poller to bring the token rules up (or confirms there are
// no clients to serve).
func (c *Client) Verify() error {
	deadline := time.Now().Add(8 * time.Second)
	var last string
	for {
		out, err := c.Run(`:put (("poller=" . [:len [/system script find where name="mkpk-tt-poller"]]) . " total=" . [/ip firewall filter print count-only where comment~"^mkpk-tt token "] . " enabled=" . [/ip firewall filter print count-only where comment~"^mkpk-tt token " disabled=no])`)
		if err != nil {
			return fmt.Errorf("verify: %w", err)
		}
		last = strings.TrimSpace(out)
		poller, total, enabled := parseVerify(last)
		if poller > 0 && (total == 0 || enabled == total) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("verify timed out: %s", last)
		}
		time.Sleep(time.Second)
	}
}

// Uninstall removes the whole mkpk-tt-* layer and mkpk globals.
func (c *Client) Uninstall() error {
	out, err := c.Run(strings.Join([]string{
		`/system scheduler remove [find where name~"^mkpk-tt-"]`,
		`/system script remove [find where name~"^mkpk-tt-"]`,
		`/ip firewall filter remove [find where comment~"^mkpk-tt"]`,
		`/ip firewall nat remove [find where comment~"^mkpk-tt "]`,
		`/ip firewall address-list remove [find where list~"^mkpk-tt-"]`,
		`/system script environment remove [find where name~"^mkpkTt"]`,
	}, "; "))
	if err != nil {
		return fmt.Errorf("uninstall: %w (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

// upload sends content to the router over SCP (RouterOS supports scp, not sftp).
func (c *Client) upload(name string, content []byte) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	if err := session.Start("scp -t " + name); err != nil {
		return err
	}
	ack := bufio.NewReader(stdout)

	errc := make(chan error, 1)
	go func() {
		if _, err := fmt.Fprintf(stdin, "C0644 %d %s\n", len(content), name); err != nil {
			errc <- err
			return
		}
		if err := readAck(ack); err != nil {
			errc <- err
			return
		}
		if _, err := stdin.Write(content); err != nil {
			errc <- err
			return
		}
		if _, err := stdin.Write([]byte{0}); err != nil {
			errc <- err
			return
		}
		if err := readAck(ack); err != nil {
			errc <- err
			return
		}
		errc <- stdin.Close()
	}()

	if err := <-errc; err != nil {
		return err
	}
	return session.Wait()
}

func readAck(r *bufio.Reader) error {
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	if b == 0 {
		return nil
	}
	// 1 = warning, 2 = fatal; the message follows up to newline.
	msg, _ := r.ReadString('\n')
	return fmt.Errorf("scp error: %s", strings.TrimSpace(msg))
}

func authMethods(a Auth) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if a.UseAgent {
		// Only add the agent when it actually holds keys; an empty agent would
		// otherwise shadow key auth and fail the handshake.
		if m := agentMethod(); m != nil {
			methods = append(methods, m)
		}
	}
	keyPath := a.KeyPath
	if keyPath == "" && a.Password == "" {
		keyPath = defaultKeyPath()
	}
	if keyPath != "" {
		m, err := keyMethod(keyPath, a.KeyPass)
		if err != nil {
			return nil, err
		}
		methods = append(methods, m)
	}
	if a.Password != "" {
		methods = append(methods, ssh.Password(a.Password))
	}
	if len(methods) == 0 {
		return nil, errors.New("no auth method: provide --key, --agent or --password")
	}
	return methods, nil
}

func agentMethod() ssh.AuthMethod {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil
	}
	signers, err := agent.NewClient(conn).Signers()
	if err != nil || len(signers) == 0 {
		return nil
	}
	return ssh.PublicKeys(signers...)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func defaultKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		p := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func keyMethod(path, pass string) (ssh.AuthMethod, error) {
	path = expandHome(path)
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key %s: %w", path, err)
	}
	var signer ssh.Signer
	if pass != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(pass))
	} else {
		signer, err = ssh.ParsePrivateKey(key)
	}
	if err != nil {
		return nil, fmt.Errorf("parse key %s: %w", path, err)
	}
	return ssh.PublicKeys(signer), nil
}

func knownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

// tofuHostKeyCallback verifies host keys against known_hosts, appending unknown
// hosts (trust-on-first-use) and rejecting changed keys.
func tofuHostKeyCallback() (ssh.HostKeyCallback, error) {
	path := knownHostsPath()
	if path == "" {
		return nil, errors.New("cannot locate ~/.ssh/known_hosts")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if f, err := os.OpenFile(path, os.O_CREATE, 0600); err == nil {
			_ = f.Close()
		}
	}
	verify, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := verify(hostname, remote, key)
		if err == nil {
			return nil
		}
		var ke *knownhosts.KeyError
		if errors.As(err, &ke) && len(ke.Want) == 0 {
			return appendKnownHost(path, hostname, remote, key)
		}
		return err
	}, nil
}

func appendKnownHost(path, hostname string, remote net.Addr, key ssh.PublicKey) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	addrs := []string{knownhosts.Normalize(hostname)}
	if remote != nil {
		if norm := knownhosts.Normalize(remote.String()); norm != addrs[0] {
			addrs = append(addrs, norm)
		}
	}
	_, err = f.WriteString(knownhosts.Line(addrs, key) + "\n")
	return err
}

func parseMetaHash(source string) string {
	for _, line := range strings.Split(source, "\n") {
		if _, hash, ok := strings.Cut(strings.TrimSpace(line), "mkpk-config-hash="); ok {
			return strings.TrimSpace(hash)
		}
	}
	return ""
}

func parseVerify(s string) (poller, total, enabled int) {
	for _, field := range strings.Fields(s) {
		name, valStr, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		val, err := strconv.Atoi(valStr)
		if err != nil {
			continue
		}
		switch name {
		case "poller":
			poller = val
		case "total":
			total = val
		case "enabled":
			enabled = val
		}
	}
	return poller, total, enabled
}
