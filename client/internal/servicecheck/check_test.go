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

func TestRunRejectsInvalidPort(t *testing.T) {
	if err := Run(Options{Host: "127.0.0.1", Port: 0}); err == nil {
		t.Fatal("Run() error = nil, want invalid port error")
	}
}
