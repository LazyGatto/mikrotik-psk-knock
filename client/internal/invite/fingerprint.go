package invite

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// An invite works only while it agrees with what is actually installed on the
// router. Change a PSK, a knock port or the router address and every invite
// already handed out dies quietly: a UDP knock cannot report delivery, so the
// user just sees "nothing opens" and the admin sees nothing at all.
//
// The fingerprint makes that comparable. It digests exactly the fields that
// must match — and nothing else, so renaming a note or changing an allowed
// timeout does not scare anyone with a false alarm.

// FingerprintLen is how many hex characters a fingerprint carries. Eight is
// enough to spot a mismatch by eye; this is a comparison aid, not a MAC.
const FingerprintLen = 8

// RouterFingerprint digests one user's access to one router: the identity that
// goes into the token, the address knocked, the bucket the token is derived
// from, the PSK, and every service's ports.
//
// The PSK goes in as an input to SHA-256 and cannot be recovered from the 8 hex
// characters that come out, so a fingerprint is safe to show in a UI or a log.
func RouterFingerprint(clientID string, r Router) string {
	svcs := append([]Service(nil), r.Services...)
	sort.Slice(svcs, func(i, j int) bool { return svcs[i].Name < svcs[j].Name })

	var sb strings.Builder
	fmt.Fprintf(&sb, "v%d|%s|%s|%d|%s", Version, clientID, r.Router, r.BucketSeconds, r.PSK)
	for _, s := range svcs {
		// Deliberately excluded: AllowedTimeout (informational) and Launch (a
		// client-side hint). Neither can break a knock.
		fmt.Fprintf(&sb, "|%s:%d:%d:%d:%d", s.Name, s.Stage1, s.Stage2, s.Token, s.CheckPort)
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return fmt.Sprintf("%x", sum)[:FingerprintLen]
}

// Fingerprint digests a whole invite — every router in it, in a stable order.
// Two invites with the same fingerprint open the same doors.
func Fingerprint(b Blob) string {
	parts := make([]string, 0, len(b.Routers))
	for _, r := range b.Routers {
		parts = append(parts, r.Router+"="+RouterFingerprint(b.ClientID, r))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", sum)[:FingerprintLen]
}
