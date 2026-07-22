package token

import (
	"testing"
	"time"
)

func TestComputeMatchesPrototypeFormula(t *testing.T) {
	got := Compute("mkpk-prototype-psk", "demo-service", "demo-client", 59490568)
	want := "9b12a9f457dc8a5a66903f58221734c09a28fdaa7f19a952b94e98c85afa59bb1a5747c5b4d46ee4766d55b889f802b3924b24d3b65055b8c6d09d25aafdeb75"
	if got != want {
		t.Fatalf("token mismatch\ngot  %s\nwant %s", got, want)
	}
}

func TestInspectWindow(t *testing.T) {
	now := time.Unix(95, 250_000_000)
	got := InspectWindow(now, 30)

	if got.Bucket != 3 {
		t.Fatalf("bucket = %d, want 3", got.Bucket)
	}
	if got.Previous != 2 {
		t.Fatalf("previous = %d, want 2", got.Previous)
	}
	if got.Next != 4 {
		t.Fatalf("next = %d, want 4", got.Next)
	}
	if got.Start.Unix() != 90 {
		t.Fatalf("start = %s, want unix 90", got.Start)
	}
	if got.End.Unix() != 120 {
		t.Fatalf("end = %s, want unix 120", got.End)
	}
	if got.Age != 5250*time.Millisecond {
		t.Fatalf("age = %s, want 5.25s", got.Age)
	}
	if got.Remaining != 24750*time.Millisecond {
		t.Fatalf("remaining = %s, want 24.75s", got.Remaining)
	}
}
