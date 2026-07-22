package knock

import (
	"fmt"
	"net"
	"time"
)

type Options struct {
	Router     string
	Stage1Port int
	Stage2Port int
	TokenPort  int
	Token      string
	Timeout    time.Duration
	Delay      time.Duration
	Logf       func(format string, args ...any)
}

func Run(opts Options) error {
	if opts.Router == "" {
		return fmt.Errorf("router address is required")
	}
	if opts.Timeout == 0 {
		opts.Timeout = time.Second
	}
	if opts.Delay == 0 {
		opts.Delay = time.Second
	}
	if err := sendUDP(opts.Router, opts.Stage1Port, []byte("x"), opts.Timeout, opts.Logf); err != nil {
		return fmt.Errorf("stage1: %w", err)
	}
	time.Sleep(opts.Delay)
	if err := sendUDP(opts.Router, opts.Stage2Port, []byte("x"), opts.Timeout, opts.Logf); err != nil {
		return fmt.Errorf("stage2: %w", err)
	}
	time.Sleep(opts.Delay)
	if err := sendUDP(opts.Router, opts.TokenPort, []byte(opts.Token), opts.Timeout, opts.Logf); err != nil {
		return fmt.Errorf("token: %w", err)
	}
	return nil
}

func sendUDP(host string, port int, payload []byte, timeout time.Duration, logf func(string, ...any)) error {
	conn, err := net.DialTimeout("udp4", net.JoinHostPort(host, fmt.Sprint(port)), timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	n, err := conn.Write(payload)
	if logf != nil {
		logf("udp local=%s remote=%s bytes=%d", conn.LocalAddr(), conn.RemoteAddr(), n)
	}
	return err
}
