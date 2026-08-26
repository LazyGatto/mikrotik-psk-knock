package deploy

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// The service account: instead of provisioning under an administrator's own
// login, mkpk gets its own user on the router with its own minimal group and
// the installation's key. Losing an admin no longer breaks deploys, and
// revoking mkpk is one user removal rather than a key hunt.
//
// Every command below was verified against a live RouterOS 7.23.2 — see
// docs/plans/2026-08-27-service-user-onboarding-plan.md for what that run
// established.

// ServiceUserName is the user and group mkpk creates on a router.
const ServiceUserName = "mkpk"

// servicePolicies is the confirmed minimal set:
//
//	ssh    log in
//	ftp    scp — RouterOS serves file upload under this policy
//	read   read back the stamped meta script, count rules
//	write  /import, firewall/scheduler/script changes, /file remove
//	policy our scripts declare policy=read,write,test, and only a user holding
//	       `policy` may assign policies
//	test   same reason, plus /tool fetch in the notification path
//
// `sensitive` is deliberately absent: a live run showed it is not needed.
const servicePolicies = "ssh,ftp,read,write,policy,test"

// ServiceUserStatus reports what exists on the router right now.
type ServiceUserStatus struct {
	UserExists  bool   `json:"user_exists"`
	GroupExists bool   `json:"group_exists"`
	KeyCount    int    `json:"key_count"`
	Policies    string `json:"policies"`
}

// ServiceUserStatus inspects the router's mkpk user, group and keys.
func (c *Client) ServiceUserStatus() (ServiceUserStatus, error) {
	out, err := c.Run(`:put ([:len [/user find where name="` + ServiceUserName + `"]] . "|" . ` +
		`[:len [/user group find where name="` + ServiceUserName + `"]] . "|" . ` +
		`[:len [/user ssh-keys find where user="` + ServiceUserName + `"]])`)
	if err != nil {
		return ServiceUserStatus{}, fmt.Errorf("service user status: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(out), "|")
	if len(parts) != 3 {
		return ServiceUserStatus{}, fmt.Errorf("service user status: unexpected output %q", strings.TrimSpace(out))
	}
	st := ServiceUserStatus{
		UserExists:  parts[0] != "0",
		GroupExists: parts[1] != "0",
	}
	_, _ = fmt.Sscanf(parts[2], "%d", &st.KeyCount)
	if st.GroupExists {
		if pol, err := c.Run(`:put [/user group get [find where name="` + ServiceUserName + `"] policy]`); err == nil {
			st.Policies = strings.TrimSpace(pol)
		}
	}
	return st, nil
}

// EnsureServiceUser creates (or refreshes) the mkpk group, user and key on the
// router. It is idempotent: re-running updates the group's policies and
// replaces the stored key rather than piling up duplicates.
//
// publicKey is an authorized_keys line — the public half of the installation
// key. The private half never leaves the provision host.
func (c *Client) EnsureServiceUser(publicKey string) error {
	publicKey = strings.TrimSpace(publicKey)
	if !strings.HasPrefix(publicKey, "ssh-") {
		return fmt.Errorf("service user: %q is not an ssh public key line", firstWord(publicKey))
	}
	if strings.ContainsAny(publicKey, "\"\n\r") {
		return fmt.Errorf("service user: public key contains characters that cannot be quoted for RouterOS")
	}

	st, err := c.ServiceUserStatus()
	if err != nil {
		return err
	}

	if st.GroupExists {
		if _, err := c.Run(`/user group set [find where name="` + ServiceUserName + `"] policy=` + servicePolicies); err != nil {
			return fmt.Errorf("service user: update group: %w", err)
		}
	} else {
		if _, err := c.Run(`/user group add name="` + ServiceUserName + `" policy=` + servicePolicies +
			` comment="mkpk provisioning service account"`); err != nil {
			return fmt.Errorf("service user: create group: %w", err)
		}
	}

	if !st.UserExists {
		// RouterOS refuses to create a user without a password, so generate one
		// and forget it: authentication happens with the key below.
		pw, err := randomPassword(32)
		if err != nil {
			return err
		}
		if _, err := c.Run(`/user add name="` + ServiceUserName + `" group="` + ServiceUserName +
			`" password="` + pw + `" comment="mkpk-provision service account"`); err != nil {
			return fmt.Errorf("service user: create user: %w", err)
		}
	} else if _, err := c.Run(`/user set [find where name="` + ServiceUserName + `"] group="` + ServiceUserName + `"`); err != nil {
		return fmt.Errorf("service user: fix group membership: %w", err)
	}

	// Replace whatever key the user had: re-onboarding after regenerating the
	// installation key must not leave the old one trusted.
	if st.KeyCount > 0 {
		if _, err := c.Run(`/user ssh-keys remove [find where user="` + ServiceUserName + `"]`); err != nil {
			return fmt.Errorf("service user: drop old keys: %w", err)
		}
	}

	// /user ssh-keys import consumes the uploaded file, so there is nothing to
	// clean up afterwards — and importing the same file twice fails.
	const keyFile = "mkpk-provision.pub"
	if err := c.upload(keyFile, []byte(publicKey+"\n")); err != nil {
		return fmt.Errorf("service user: upload key: %w", err)
	}
	if out, err := c.Run(`/user ssh-keys import user="` + ServiceUserName + `" public-key-file="` + keyFile + `"`); err != nil {
		_, _ = c.Run(`/file remove [find where name="` + keyFile + `"]`)
		return fmt.Errorf("service user: import key: %w (%s)", err, strings.TrimSpace(out))
	}

	after, err := c.ServiceUserStatus()
	if err != nil {
		return err
	}
	if !after.UserExists || after.KeyCount == 0 {
		return fmt.Errorf("service user: router reports user=%t keys=%d after onboarding", after.UserExists, after.KeyCount)
	}
	return nil
}

// SSHPasswordAuth reads /ip ssh password-authentication. RouterOS defaults to
// "yes-if-no-key": a user with an imported key can no longer log in with a
// password. Admins often flip it to "yes" to let provisioning in, and then
// forget to flip it back.
func (c *Client) SSHPasswordAuth() (string, error) {
	out, err := c.Run(`:put [/ip ssh get password-authentication]`)
	if err != nil {
		return "", fmt.Errorf("read ssh password-authentication: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// SetSSHPasswordAuth writes that setting back.
func (c *Client) SetSSHPasswordAuth(value string) error {
	switch value {
	case "yes", "no", "yes-if-no-key":
	default:
		return fmt.Errorf("ssh password-authentication: unexpected value %q", value)
	}
	if _, err := c.Run(`/ip ssh set password-authentication=` + value); err != nil {
		return fmt.Errorf("set ssh password-authentication=%s: %w", value, err)
	}
	return nil
}

// RemoveServiceUser deletes the key, the user and the group in one command.
// A live check showed RouterOS runs the whole line before tearing down the
// session, so the service user can remove itself — no admin credentials needed
// to hand a router back.
func (c *Client) RemoveServiceUser() error {
	cmd := `/user ssh-keys remove [find where user="` + ServiceUserName + `"]; ` +
		`/user remove [find where name="` + ServiceUserName + `"]; ` +
		`/user group remove [find where name="` + ServiceUserName + `"]`
	// The session dies together with the user, so a transport-level error here
	// is expected rather than a failure; the caller verifies with a fresh
	// connection when it can.
	_, _ = c.Run(cmd)
	return nil
}

// RemoveServiceUserCommand returns the same command for a human to paste when
// the router is unreachable and the account has to be cleaned up by hand.
func RemoveServiceUserCommand() string {
	return `/user ssh-keys remove [find where user="` + ServiceUserName + `"]; ` +
		`/user remove [find where name="` + ServiceUserName + `"]; ` +
		`/user group remove [find where name="` + ServiceUserName + `"]`
}

// randomPassword builds a password RouterOS accepts on /user add. It is never
// stored or returned: the account authenticates with its key.
func randomPassword(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var sb strings.Builder
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		sb.WriteByte(alphabet[idx.Int64()])
	}
	return sb.String(), nil
}

func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t"); i > 0 {
		return s[:i]
	}
	if len(s) > 24 {
		return s[:24] + "…"
	}
	return s
}
