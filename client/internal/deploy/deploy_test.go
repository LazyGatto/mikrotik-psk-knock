package deploy

import "testing"

func TestParseMetaHash(t *testing.T) {
	src := "# mkpk-version=1\n# mkpk-config-hash=abc123def456\n"
	if got := parseMetaHash(src); got != "abc123def456" {
		t.Fatalf("parseMetaHash = %q, want abc123def456", got)
	}
	if got := parseMetaHash("# no hash here"); got != "" {
		t.Fatalf("parseMetaHash = %q, want empty", got)
	}
}

func TestParseVerify(t *testing.T) {
	poller, total, enabled := parseVerify("poller=1 total=4 enabled=4")
	if poller != 1 || total != 4 || enabled != 4 {
		t.Fatalf("parseVerify = %d/%d/%d, want 1/4/4", poller, total, enabled)
	}

	poller, total, enabled = parseVerify("poller=0 total=0 enabled=0")
	if poller != 0 || total != 0 || enabled != 0 {
		t.Fatalf("parseVerify = %d/%d/%d, want 0/0/0", poller, total, enabled)
	}
}
