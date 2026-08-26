package deploy

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestPasswordAuthAlsoOffersKeyboardInteractive(t *testing.T) {
	// RouterOS builds differ in which of the two password methods they
	// advertise, so a password must produce both.
	methods, err := authMethods(Auth{User: "admin", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 2 {
		t.Fatalf("got %d methods for a password, want password + keyboard-interactive", len(methods))
	}
}

func TestKeyboardInteractiveAnswersWithThePassword(t *testing.T) {
	methods, err := authMethods(Auth{User: "admin", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ki, ok := methods[1].(ssh.KeyboardInteractiveChallenge)
	if !ok {
		t.Fatalf("second method is %T, want a keyboard-interactive challenge", methods[1])
	}
	answers, err := ki("", "", []string{"Password: ", "Password again: "}, []bool{false, false})
	if err != nil {
		t.Fatal(err)
	}
	if len(answers) != 2 || answers[0] != "secret" || answers[1] != "secret" {
		t.Fatalf("answers = %q, want the password for every prompt", answers)
	}
}

func TestAuthHintExplainsTheRouterOSDefault(t *testing.T) {
	err := errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password]")
	hint := routerOSAuthHint(err, Auth{Password: "x"})
	if !strings.Contains(hint, "yes-if-no-key") {
		t.Fatalf("password hint does not mention the RouterOS default: %q", hint)
	}
	if h := routerOSAuthHint(err, Auth{KeyPath: "/k"}); !strings.Contains(h, "ssh-keys print") {
		t.Fatalf("key hint should point at the router key list: %q", h)
	}
	// Unrelated failures must not collect advice they cannot use.
	if h := routerOSAuthHint(errors.New("dial tcp: i/o timeout"), Auth{Password: "x"}); h != "" {
		t.Fatalf("hint on a network error: %q", h)
	}
}
