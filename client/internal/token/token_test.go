package token

import "testing"

func TestComputeMatchesPrototypeFormula(t *testing.T) {
	got := Compute("mkpk-prototype-psk", "demo-service", "demo-client", 59490568)
	want := "9b12a9f457dc8a5a66903f58221734c09a28fdaa7f19a952b94e98c85afa59bb1a5747c5b4d46ee4766d55b889f802b3924b24d3b65055b8c6d09d25aafdeb75"
	if got != want {
		t.Fatalf("token mismatch\ngot  %s\nwant %s", got, want)
	}
}
