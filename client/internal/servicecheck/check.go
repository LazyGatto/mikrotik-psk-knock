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

type Result struct {
	Status    string
	Host      string
	Port      int
	Attempts  int
	Duration  time.Duration
	LastError string
	lastErr   error
}

func Run(opts Options) error {
	result := Check(opts)
	if result.lastErr == nil {
		return nil
	}
	return result.lastErr
}

func Check(opts Options) Result {
	started := time.Now()
	result := Result{
		Status: "error",
		Host:   opts.Host,
		Port:   opts.Port,
	}
	if opts.Host == "" {
		return result.withError(started, fmt.Errorf("host is required"))
	}
	if opts.Port <= 0 || opts.Port > 65535 {
		return result.withError(started, fmt.Errorf("port must be between 1 and 65535"))
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
		return result.withError(started, fmt.Errorf("attempts must be non-negative"))
	}
	if opts.Interval < 0 {
		return result.withError(started, fmt.Errorf("interval must be non-negative"))
	}
	if opts.Timeout < 0 {
		return result.withError(started, fmt.Errorf("timeout must be non-negative"))
	}

	addr := net.JoinHostPort(opts.Host, fmt.Sprint(opts.Port))
	var lastErr error
	for attempt := 1; attempt <= opts.Attempts; attempt++ {
		result.Attempts = attempt
		conn, err := net.DialTimeout("tcp", addr, opts.Timeout)
		if err == nil {
			_ = conn.Close()
			if opts.Logf != nil {
				opts.Logf("check tcp attempt=%d remote=%s status=open", attempt, addr)
			}
			result.Status = "open"
			result.Attempts = attempt
			result.Duration = time.Since(started)
			return result
		}
		lastErr = err
		if opts.Logf != nil {
			opts.Logf("check tcp attempt=%d remote=%s status=closed error=%v", attempt, addr, err)
		}
		if attempt < opts.Attempts {
			time.Sleep(opts.Interval)
		}
	}
	return result.withError(started, fmt.Errorf("tcp check failed for %s after %d attempts: %w", addr, opts.Attempts, lastErr))
}

func (r Result) withError(started time.Time, err error) Result {
	if r.Attempts == 0 {
		r.Attempts = 0
	}
	r.Status = "closed"
	if r.Host == "" || r.Port <= 0 || r.Port > 65535 {
		r.Status = "error"
	}
	r.Duration = time.Since(started)
	r.LastError = err.Error()
	r.lastErr = err
	return r
}
