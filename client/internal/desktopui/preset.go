package desktopui

import (
	"fmt"
	"runtime"
	"strings"
)

// presetLine turns an invite's launch KIND into a command line for this OS,
// built from the router address and port the client already holds. The invite
// never supplies command text — see invite.Service.Launch for why.
//
// host/port are validated first: an invite is untrusted input, and these values
// end up inside a shell line. A rejected host means no launch at all.
func presetLine(kind, host string, port int) (string, error) {
	if kind == "" {
		return "", nil
	}
	if !isSafeHost(host) {
		return "", fmt.Errorf("launch: refusing to open %q — not a plain host or IP", host)
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("launch: port %d out of range", port)
	}
	target := fmt.Sprintf("%s:%d", host, port)

	if runtime.GOOS == "windows" {
		switch kind {
		case "rdp":
			return `start "" mstsc /v:` + target, nil
		case "ssh":
			// A GUI process has no console, so ssh needs its own window.
			return fmt.Sprintf(`start "" cmd /k ssh -p %d %s`, port, host), nil
		case "http", "https", "vnc":
			return fmt.Sprintf(`start "" %s://%s`, kind, target), nil
		}
		return "", fmt.Errorf("launch: unknown kind %q", kind)
	}

	// macOS / Linux dev runs: hand the URL to the desktop opener.
	switch kind {
	case "rdp":
		return fmt.Sprintf(`open "rdp://full%%20address=s:%s"`, target), nil
	case "ssh":
		return fmt.Sprintf(`open "ssh://%s"`, target), nil
	case "http", "https", "vnc":
		return fmt.Sprintf(`open "%s://%s"`, kind, target), nil
	}
	return "", fmt.Errorf("launch: unknown kind %q", kind)
}

// isSafeHost accepts a hostname or IPv4 literal — letters, digits, dot, dash —
// and nothing that could change the meaning of a shell line (quotes, &, |, ;,
// spaces, backticks, $).
func isSafeHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if strings.HasPrefix(host, "-") || strings.HasSuffix(host, "-") ||
		strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '-':
		default:
			return false
		}
	}
	return true
}
