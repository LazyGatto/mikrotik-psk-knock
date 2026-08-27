package admin

import (
	"sort"
	"strings"

	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/invite"
)

// An invite already in someone's hands keeps working only while the config
// still describes the same doors. Change a PSK, a knock port, the router
// address or the bucket, and that invite is dead — silently, because a UDP
// knock cannot report delivery. The admin gets no signal at all today, and the
// break surfaces at the next deploy, often days later and next to an unrelated
// change.
//
// So: the same fingerprint the client can print, computed from the config.

// AccessFingerprint is the invite fingerprint for one (user, router) pair, or
// "" when that pair has nothing to export (no access, no enabled services).
func AccessFingerprint(cfg config.Config, userName, routerName string) string {
	b, err := buildInviteBlob(cfg, userName, routerName)
	if err != nil || len(b.Routers) != 1 {
		return ""
	}
	return invite.RouterFingerprint(b.ClientID, b.Routers[0])
}

// fingerprintKey identifies one (user, router) pair in a fingerprint map.
func fingerprintKey(userName, routerName string) string { return userName + "/" + routerName }

// Fingerprints maps every (user, router) pair in the config to its invite
// fingerprint. Pairs with nothing to export are omitted.
func Fingerprints(cfg config.Config) map[string]string {
	out := map[string]string{}
	for un, u := range cfg.Users {
		for rn := range u.Access {
			if fp := AccessFingerprint(cfg, un, rn); fp != "" {
				out[fingerprintKey(un, rn)] = fp
			}
		}
	}
	return out
}

// InvalidatedInvites compares the fingerprints of two configs and returns the
// users whose already-issued invites the change just broke: their access still
// exists but no longer matches. Users who simply lost access are not listed —
// that is the admin doing it on purpose, and the invite is meant to stop
// working.
func InvalidatedInvites(before, after config.Config) []string {
	old := Fingerprints(before)
	now := Fingerprints(after)
	seen := map[string]bool{}
	var users []string
	for key, fp := range old {
		cur, still := now[key]
		if !still || cur == fp {
			continue
		}
		user := key[:strings.Index(key, "/")]
		if !seen[user] {
			seen[user] = true
			users = append(users, user)
		}
	}
	sort.Strings(users)
	return users
}
