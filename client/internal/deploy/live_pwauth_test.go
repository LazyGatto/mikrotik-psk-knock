package deploy

import (
	"os"
	"testing"
)

// TestLivePasswordAuthRoundTrip talks to a real router when MKPK_LIVE_ROUTER is
// set; skipped otherwise, so the gate stays offline-safe. It writes back the
// value it read, so it verifies the commands without changing the router.
func TestLivePasswordAuthRoundTrip(t *testing.T) {
	host := os.Getenv("MKPK_LIVE_ROUTER")
	if host == "" {
		t.Skip("MKPK_LIVE_ROUTER not set")
	}
	c, err := Connect(host, 22, Auth{User: os.Getenv("MKPK_LIVE_USER"), KeyPath: os.Getenv("MKPK_LIVE_KEY")})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	before, err := c.SSHPasswordAuth()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	t.Logf("password-authentication = %q", before)
	if before != "yes" && before != "no" && before != "yes-if-no-key" {
		t.Fatalf("unexpected value %q", before)
	}
	if err := c.SetSSHPasswordAuth(before); err != nil {
		t.Fatalf("write back the same value: %v", err)
	}
	after, err := c.SSHPasswordAuth()
	if err != nil || after != before {
		t.Fatalf("value changed: %q → %q (err %v)", before, after, err)
	}
	if err := c.SetSSHPasswordAuth("nonsense"); err == nil {
		t.Fatal("a bogus value was accepted")
	}
}
