package servicecheck

import (
	"net"
	"testing"
	"time"
)

func TestRunSucceedsWhenPortIsOpen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	if err := Run(Options{
		Host:     "127.0.0.1",
		Port:     port,
		Timeout:  time.Second,
		Attempts: 1,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("listener did not accept the check connection")
	}
}

func TestCheckReturnsOpenResult(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	got := Check(Options{
		Host:     "127.0.0.1",
		Port:     port,
		Timeout:  time.Second,
		Attempts: 1,
	})

	if got.Status != "open" {
		t.Fatalf("status = %q, want open", got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", got.Attempts)
	}
	if got.LastError != "" {
		t.Fatalf("last error = %q, want empty", got.LastError)
	}
}

func TestCheckReturnsClosedResult(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	got := Check(Options{
		Host:     "127.0.0.1",
		Port:     port,
		Timeout:  100 * time.Millisecond,
		Attempts: 1,
	})

	if got.Status != "closed" {
		t.Fatalf("status = %q, want closed", got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", got.Attempts)
	}
	if got.LastError == "" {
		t.Fatal("last error is empty, want connection error")
	}
}

func TestRunRejectsInvalidPort(t *testing.T) {
	if err := Run(Options{Host: "127.0.0.1", Port: 0}); err == nil {
		t.Fatal("Run() error = nil, want invalid port error")
	}
}
