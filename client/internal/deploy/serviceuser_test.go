package deploy

import (
	"strings"
	"testing"
)

func TestServicePoliciesMatchWhatWasVerified(t *testing.T) {
	// The set was established on a live RouterOS 7.23.2; widening it silently
	// would defeat the point of a least-privilege service account.
	want := map[string]bool{"ssh": true, "ftp": true, "read": true, "write": true, "policy": true, "test": true}
	got := strings.Split(servicePolicies, ",")
	if len(got) != len(want) {
		t.Fatalf("policies = %q, want exactly %d entries", servicePolicies, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected policy %q — widening needs a live re-check", p)
		}
	}
	if strings.Contains(servicePolicies, "sensitive") {
		t.Error("sensitive is not needed and must not be granted")
	}
}

func TestRemoveCommandOrder(t *testing.T) {
	cmd := RemoveServiceUserCommand()
	keys := strings.Index(cmd, "ssh-keys remove")
	user := strings.Index(cmd, "/user remove")
	group := strings.Index(cmd, "/user group remove")
	if keys < 0 || user < 0 || group < 0 {
		t.Fatalf("command misses a step: %s", cmd)
	}
	// The user removes itself, which ends the session — so its own removal must
	// come after the key and before nothing else matters; the group follows in
	// the same line, which RouterOS still executes.
	if !(keys < user && user < group) {
		t.Fatalf("wrong order (keys=%d user=%d group=%d): %s", keys, user, group, cmd)
	}
	if !strings.Contains(cmd, ServiceUserName) {
		t.Fatalf("command does not mention %q: %s", ServiceUserName, cmd)
	}
}

func TestRandomPasswordIsUsableAndUnique(t *testing.T) {
	a, err := randomPassword(32)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := randomPassword(32)
	if a == b {
		t.Fatal("two generated passwords are identical")
	}
	if len(a) != 32 {
		t.Fatalf("length = %d, want 32", len(a))
	}
	// RouterOS quoting: the password is interpolated into a command, so it must
	// stay alphanumeric.
	for _, r := range a {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			t.Fatalf("password contains %q, which would need quoting", r)
		}
	}
}
