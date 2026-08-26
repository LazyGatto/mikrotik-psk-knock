package desktopui

import (
	"runtime"
	"strings"
	"testing"
)

func TestPresetLineBuildsFromHostPort(t *testing.T) {
	line, err := presetLine("rdp", "kz.example.com", 3389)
	if err != nil {
		t.Fatalf("rdp: %v", err)
	}
	if !strings.Contains(line, "kz.example.com") || !strings.Contains(line, "3389") {
		t.Fatalf("rdp line lacks host/port: %q", line)
	}
	if runtime.GOOS == "windows" && !strings.Contains(line, "mstsc") {
		t.Fatalf("windows rdp line = %q, want mstsc", line)
	}
	if empty, err := presetLine("", "h", 1); empty != "" || err != nil {
		t.Fatalf("empty kind = %q, %v; want no line, no error", empty, err)
	}
	if _, err := presetLine("nonsense", "h", 1); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

// TestPresetRejectsHostileHost is the point of the preset design: an invite is
// untrusted, so a host carrying shell metacharacters must never reach a shell.
func TestPresetRejectsHostileHost(t *testing.T) {
	hostile := []string{
		`h & calc.exe`,
		`h" & calc & "`,
		"h;rm -rf ~",
		"h|nc attacker 1",
		"h`id`",
		"h$(id)",
		"h space",
		"",
		"-h",
	}
	for _, host := range hostile {
		if line, err := presetLine("rdp", host, 3389); err == nil {
			t.Errorf("host %q accepted, produced %q", host, line)
		}
	}
	for _, ok := range []string{"kz.d2a.ru", "192.0.2.10", "router-1.example.com"} {
		if _, err := presetLine("rdp", ok, 3389); err != nil {
			t.Errorf("legit host %q rejected: %v", ok, err)
		}
	}
	if _, err := presetLine("rdp", "h.example.com", 70000); err == nil {
		t.Error("out-of-range port accepted")
	}
}
