package knock

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"time"
)

type Options struct {
	Router        string
	Stage1Port    int
	Stage2Port    int
	TokenPort     int
	Token         string
	Timeout       time.Duration
	Interval      time.Duration
	StageDuration time.Duration
	TokenDuration time.Duration
	NoisePackets  int
	Logf          func(format string, args ...any)
}

func Run(opts Options) error {
	if opts.Router == "" {
		return fmt.Errorf("router address is required")
	}
	if opts.Timeout == 0 {
		opts.Timeout = time.Second
	}
	if opts.Interval == 0 {
		opts.Interval = 250 * time.Millisecond
	}
	if opts.StageDuration == 0 {
		opts.StageDuration = 2 * time.Second
	}
	if opts.TokenDuration == 0 {
		opts.TokenDuration = time.Second
	}
	if err := sendNoise(opts, "pre-stage"); err != nil {
		return err
	}
	if err := sendRepeated(opts, "stage1", opts.Stage1Port, []byte("x"), opts.StageDuration); err != nil {
		return fmt.Errorf("stage1: %w", err)
	}
	if err := sendNoise(opts, "post-stage1"); err != nil {
		return err
	}
	if err := sendRepeated(opts, "stage2", opts.Stage2Port, []byte("x"), opts.StageDuration); err != nil {
		return fmt.Errorf("stage2: %w", err)
	}
	if err := sendNoise(opts, "post-stage2"); err != nil {
		return err
	}
	if err := sendRepeated(opts, "token", opts.TokenPort, []byte(opts.Token), opts.TokenDuration); err != nil {
		return fmt.Errorf("token: %w", err)
	}
	return nil
}

func sendRepeated(opts Options, label string, port int, payload []byte, duration time.Duration) error {
	deadline := time.Now().Add(duration)
	for attempt := 1; ; attempt++ {
		if err := sendUDP(opts.Router, port, payload, opts.Timeout, opts.Logf, label, attempt); err != nil {
			return err
		}
		if !time.Now().Add(opts.Interval).Before(deadline) {
			return nil
		}
		time.Sleep(opts.Interval)
	}
}

func sendNoise(opts Options, label string) error {
	for i := 0; i < opts.NoisePackets; i++ {
		payload, err := randomPayload()
		if err != nil {
			return fmt.Errorf("%s noise: %w", label, err)
		}
		if err := sendUDP(opts.Router, opts.TokenPort, payload, opts.Timeout, opts.Logf, label+"-noise", i+1); err != nil {
			return fmt.Errorf("%s noise: %w", label, err)
		}
	}
	return nil
}

func randomPayload() ([]byte, error) {
	nBig, err := rand.Int(rand.Reader, big.NewInt(48))
	if err != nil {
		return nil, err
	}
	payload := make([]byte, 16+nBig.Int64())
	if _, err := rand.Read(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func sendUDP(host string, port int, payload []byte, timeout time.Duration, logf func(string, ...any), label string, attempt int) error {
	conn, err := net.DialTimeout("udp4", net.JoinHostPort(host, fmt.Sprint(port)), timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	n, err := conn.Write(payload)
	if logf != nil {
		logf("udp phase=%s attempt=%d local=%s remote=%s bytes=%d", label, attempt, conn.LocalAddr(), conn.RemoteAddr(), n)
	}
	return err
}
