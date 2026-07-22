package servicecheck

import (
	"fmt"
	"net"
	"time"
)

type Options struct {
	Host     string
	Port     int
	Timeout  time.Duration
	Attempts int
	Interval time.Duration
	Logf     func(format string, args ...any)
}

func Run(opts Options) error {
	if opts.Host == "" {
		return fmt.Errorf("host is required")
	}
	if opts.Port <= 0 || opts.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if opts.Timeout == 0 {
		opts.Timeout = time.Second
	}
	if opts.Attempts == 0 {
		opts.Attempts = 10
	}
	if opts.Interval == 0 {
		opts.Interval = 500 * time.Millisecond
	}
	if opts.Attempts < 0 {
		return fmt.Errorf("attempts must be non-negative")
	}
	if opts.Interval < 0 {
		return fmt.Errorf("interval must be non-negative")
	}
	if opts.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative")
	}

	addr := net.JoinHostPort(opts.Host, fmt.Sprint(opts.Port))
	var lastErr error
	for attempt := 1; attempt <= opts.Attempts; attempt++ {
		conn, err := net.DialTimeout("tcp", addr, opts.Timeout)
		if err == nil {
			_ = conn.Close()
			if opts.Logf != nil {
				opts.Logf("check tcp attempt=%d remote=%s status=open", attempt, addr)
			}
			return nil
		}
		lastErr = err
		if opts.Logf != nil {
			opts.Logf("check tcp attempt=%d remote=%s status=closed error=%v", attempt, addr, err)
		}
		if attempt < opts.Attempts {
			time.Sleep(opts.Interval)
		}
	}
	return fmt.Errorf("tcp check failed for %s after %d attempts: %w", addr, opts.Attempts, lastErr)
}
