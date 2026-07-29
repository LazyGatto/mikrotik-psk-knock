package admin

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/knock"
	"mikrotik-psk-knock/client/internal/servicecheck"
	"mikrotik-psk-knock/client/internal/token"
)

// TestResult is the outcome of an end-to-end knock test against a deployed
// router: it knocks from this host, then reads the router-side firewall counters,
// allowed-list and the target port over SSH to see exactly how far the sequence
// got. This is the honest UDP-delivery check the client can't do (it needs SSH).
type TestResult struct {
	Router      string `json:"router"`
	Service     string `json:"service"`
	Client      string `json:"client"`
	Bucket      int64  `json:"bucket"`
	Stage1Delta int    `json:"stage1_delta"`
	Stage2Delta int    `json:"stage2_delta"`
	TokenDelta  int    `json:"token_delta"` // now + prev token rules combined
	RulesFound  bool   `json:"rules_found"` // the mkpk-tt rules exist on the router
	AllowedHit  bool   `json:"allowed_hit"` // an entry appeared in the service allowed-list
	PortOpen    bool   `json:"port_open"`   // TCP connect to check_port succeeded
	Pass        bool   `json:"pass"`
	Diagnosis   string `json:"diagnosis"`
}

// KnockTest runs the full knock→verify cycle for one (router, service, client).
// It streams human-readable progress to logf (nil = quiet). o carries optional
// SSH overrides; o.OnLog receives the raw SSH transcript lines live.
func KnockTest(cfg config.Config, routerName, serviceName, clientName string, wait time.Duration, o DeployOptions, logf func(string, ...any)) (TestResult, error) {
	log := func(format string, args ...any) {
		if logf != nil {
			logf(format, args...)
		}
	}
	if wait <= 0 {
		wait = 4 * time.Second
	}

	res, err := cfg.Resolve(clientName, routerName, serviceName)
	if err != nil {
		return TestResult{}, err
	}
	out := TestResult{Router: res.RouterName, Service: res.ServiceName, Client: res.UserName}

	win := token.InspectWindow(time.Now(), res.Router.Defaults.BucketSeconds)
	tok := token.Compute(res.PSK, res.Service.ServiceName, res.ClientID, win.Bucket)
	out.Bucket = win.Bucket
	log("Клиент %q, сервис %q, bucket=%d (bucket_seconds=%d)", res.ClientID, res.ServiceName, win.Bucket, res.Router.Defaults.BucketSeconds)

	svcKey := res.ServiceName
	pairKey := res.UserName + "-" + res.ServiceName
	stage1Comment := "mkpk-tt stage1 " + svcKey
	stage2Comment := "mkpk-tt stage2 " + svcKey
	tokenNowComment := "mkpk-tt token now " + pairKey
	tokenPrevComment := "mkpk-tt token prev " + pairKey

	c, addr, err := connect(res.Router, o)
	if err != nil {
		return out, fmt.Errorf("SSH: %w", err)
	}
	defer c.Close()
	log("SSH подключён: %s", addr)

	// Baseline counters.
	s1a, found1, err := ruleCounter(c, stage1Comment)
	if err != nil {
		return out, err
	}
	s2a, _, err := ruleCounter(c, stage2Comment)
	if err != nil {
		return out, err
	}
	tNowA, foundTok, err := ruleCounter(c, tokenNowComment)
	if err != nil {
		return out, err
	}
	tPrevA, _, err := ruleCounter(c, tokenPrevComment)
	if err != nil {
		return out, err
	}
	out.RulesFound = found1 && foundTok
	if !out.RulesFound {
		out.Diagnosis = "Правила mkpk-tt для этого сервиса/клиента не найдены на роутере — нужен Deploy."
		log("✗ %s", out.Diagnosis)
		return out, nil
	}
	// "Allow granted" is detected via a fresh log line for OUR client key — the
	// allowed-list count can stay flat when our IP already had an entry (a knock
	// refreshes its TTL rather than adding a row).
	allowA := allowLogCount(c, pairKey)
	log("Базовые счётчики: stage1=%d stage2=%d token(now/prev)=%d/%d allow-лог=%d", s1a, s2a, tNowA, tPrevA, allowA)

	// Knock from this host.
	log("Стучу stage1→stage2→token на %s…", res.Router.Address)
	if err := knock.Run(knock.Options{
		Router:     res.Router.Address,
		Stage1Port: res.Service.Stage1Port,
		Stage2Port: res.Service.Stage2Port,
		TokenPort:  res.Service.TokenPort,
		Token:      tok,
		Logf:       logf,
	}); err != nil {
		return out, fmt.Errorf("knock: %w", err)
	}

	log("Жду %s (poller применяет токен → allowed-list)…", wait)
	time.Sleep(wait)

	// Counters after.
	s1b, _, _ := ruleCounter(c, stage1Comment)
	s2b, _, _ := ruleCounter(c, stage2Comment)
	tNowB, _, _ := ruleCounter(c, tokenNowComment)
	tPrevB, _, _ := ruleCounter(c, tokenPrevComment)
	allowB := allowLogCount(c, pairKey)

	out.Stage1Delta = s1b - s1a
	out.Stage2Delta = s2b - s2a
	out.TokenDelta = (tNowB + tPrevB) - (tNowA + tPrevA)
	out.AllowedHit = allowB > allowA
	log("Дельты: stage1=+%d stage2=+%d token=+%d allow-лог=+%d", out.Stage1Delta, out.Stage2Delta, out.TokenDelta, allowB-allowA)

	if tail := logTail(c); tail != "" {
		log("Лог роутера (mkpk-tt allowed):\n%s", tail)
	}

	// TCP check to the service's check port, from this host (same source IP).
	checkPort := res.Service.Target.Port
	if checkPort > 0 {
		log("TCP-проверка %s:%d…", res.Router.Address, checkPort)
		r := servicecheck.Check(servicecheck.Options{
			Host: res.Router.Address, Port: checkPort,
			Timeout: 2 * time.Second, Attempts: 5, Interval: 500 * time.Millisecond,
		})
		out.PortOpen = r.Status == "open"
	}

	out.Pass, out.Diagnosis = diagnose(out, checkPort)
	if out.Pass {
		log("✓ PASS — %s", out.Diagnosis)
	} else {
		log("✗ FAIL — %s", out.Diagnosis)
	}
	return out, nil
}

// diagnose turns the counter deltas / port state into a verdict pinpointing the
// first broken step.
func diagnose(r TestResult, checkPort int) (bool, string) {
	switch {
	case r.Stage1Delta == 0:
		return false, "UDP не доходит до роутера: счётчик stage1 не изменился (порт stage1 режется по пути или роутер недоступен по UDP)."
	case r.Stage2Delta == 0:
		return false, "stage1 получен, а stage2 — нет (проверь порт stage2 / фильтрацию между стадиями)."
	case r.TokenDelta == 0:
		return false, "stage1+stage2 ок, но токен не сматчил (часы роутера разошлись / poller не загрузил токен / неверный PSK или service_name)."
	}
	// Token matched. The TCP check is the ultimate signal when a check port exists.
	if checkPort > 0 {
		if r.PortOpen {
			return true, "Стук прошёл: токен сматчил, доступ выдан, порт открыт."
		}
		if r.AllowedHit {
			return false, "Доступ выдан (лог allowed), но порт закрыт по TCP (за портом никто не слушает / dst-nat?)."
		}
		return false, "Токен сматчил, но ни allowed-лога, ни открытого порта (apply-service/poller?)."
	}
	if r.AllowedHit {
		return true, "Стук прошёл: токен сматчил, доступ выдан (check_port не задан)."
	}
	return false, "Токен сматчил, но allow не выдан (apply-service/poller?)."
}

// ruleCounter returns the packet counter of the firewall filter rule with the
// given comment. found=false means no such rule exists.
func ruleCounter(c interface{ Run(string) (string, error) }, comment string) (count int, found bool, err error) {
	cmd := fmt.Sprintf(`:local id [/ip firewall filter find where comment="%s"]; :if ([:len $id]>0) do={:put [/ip firewall filter get $id packets]} else={:put "NORULE"}`, comment)
	out, err := c.Run(cmd)
	if err != nil {
		return 0, false, err
	}
	s := strings.TrimSpace(out)
	if s == "" || s == "NORULE" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false, fmt.Errorf("parse counter %q: %w", s, err)
	}
	return n, true, nil
}

// allowLogCount counts the router log lines recording a granted allow for this
// exact client key ("mkpk-tt allowed ... client=<pairKey>"). A delta > 0 across
// the knock means access was (re)granted for us — robust to the allowed-list row
// already existing (a knock refreshes its TTL, leaving the row count unchanged).
func allowLogCount(c interface{ Run(string) (string, error) }, pairKey string) int {
	out, err := c.Run(fmt.Sprintf(`:put [/log print count-only where message~"mkpk-tt allowed.*client=%s"]`, pairKey))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0
	}
	return n
}

// logTail returns the last few mkpk-tt allowed log lines, for the transcript.
func logTail(c interface{ Run(string) (string, error) }) string {
	out, err := c.Run(`/log print where message~"mkpk-tt allowed"`)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 4 {
		lines = lines[len(lines)-4:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
