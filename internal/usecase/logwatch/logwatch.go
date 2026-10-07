// Package logwatch turns dropbear and LuCI log lines into login events and
// rate-limits them per address.
package logwatch

import (
	"context"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
	"github.com/emgeorrk/wrtgram/pkg/throttle"
)

// Options tune the watcher.
type Options struct {
	TrustedIPs    []string // successes from these addresses are not reported
	SuccessWindow time.Duration
	FailureWindow time.Duration
}

// Watcher consumes the log stream and emits throttled login events.
type Watcher struct {
	source  usecase.LogSource
	clock   usecase.Clock
	limiter *throttle.Limiter
	trusted map[string]bool
	opts    Options
}

// New creates the watcher.
func New(source usecase.LogSource, clock usecase.Clock, opts Options) *Watcher {
	trusted := make(map[string]bool, len(opts.TrustedIPs))
	for _, ip := range opts.TrustedIPs {
		trusted[ip] = true
	}

	return &Watcher{source: source, clock: clock, limiter: throttle.New(), trusted: trusted, opts: opts}
}

// Run blocks, calling emit for every event that passes the throttle.
func (w *Watcher) Run(ctx context.Context, emit func(entity.LoginEvent)) error {
	lines, err := w.source.Tail(ctx)
	if err != nil {
		return err
	}

	for line := range lines {
		ev, ok := Parse(line)
		if !ok || !w.allow(ev) {
			continue
		}

		emit(ev)
	}

	return nil
}

func (w *Watcher) allow(ev entity.LoginEvent) bool {
	if ev.Success && w.trusted[ev.IP] {
		return false
	}

	kind, window := "fail", w.opts.FailureWindow
	if ev.Success {
		kind, window = "ok", w.opts.SuccessWindow
	}

	key := strings.Join([]string{surface(ev.Kind), kind, ev.IP}, ":")

	return w.limiter.Allow(key, window, w.clock.Now())
}

func surface(k entity.LoginKind) string {
	if k == entity.LoginLuCI {
		return "luci"
	}

	return "ssh"
}

// Parse recognizes the authentication lines of dropbear and LuCI.
func Parse(line entity.LogLine) (entity.LoginEvent, bool) {
	switch {
	case line.Tag == "dropbear":
		return parseDropbear(line)
	case strings.HasPrefix(line.Message, "luci: "):
		return parseLuCI(line)
	}

	return entity.LoginEvent{}, false
}

// dropbear messages (src/svr-auth.c, svr-authpasswd.c, svr-authpubkey.c):
//
//	Password auth succeeded for 'root' from 192.168.1.2:50661
//	Pubkey auth succeeded for 'root' with ssh-ed25519 key SHA256:… from 192.168.1.2:50661
//	Bad password attempt for 'root' from 192.168.1.2:50661
//	Login attempt for nonexistent user from 192.168.1.2:50661
//	Login attempt with wrong user admin from 192.168.1.2:50661
func parseDropbear(line entity.LogLine) (entity.LoginEvent, bool) {
	ev := entity.LoginEvent{At: line.At, Kind: entity.LoginSSH}
	msg := line.Message

	switch {
	case strings.HasPrefix(msg, "Password auth succeeded for "):
		ev.Success, ev.Method = true, "password"
	case strings.HasPrefix(msg, "Pubkey auth succeeded for "):
		ev.Success, ev.Method = true, "key"
	case strings.HasPrefix(msg, "Bad password attempt for "),
		strings.HasPrefix(msg, "Login attempt for nonexistent user"),
		strings.HasPrefix(msg, "Login attempt with wrong user "):
	default:
		return entity.LoginEvent{}, false
	}

	ev.User = quoted(msg)
	ev.IP = stripPort(after(msg, " from "))

	return ev, ev.IP != ""
}

// LuCI messages (modules/luci-base/ucode/dispatcher.uc):
//
//	luci: accepted login on /admin for root from 192.168.1.2
//	luci: failed login on /admin for root from 192.168.1.2
func parseLuCI(line entity.LogLine) (entity.LoginEvent, bool) {
	ev := entity.LoginEvent{At: line.At, Kind: entity.LoginLuCI}
	msg := strings.TrimPrefix(line.Message, "luci: ")

	switch {
	case strings.HasPrefix(msg, "accepted login"):
		ev.Success = true
	case strings.HasPrefix(msg, "failed login"):
	default:
		return entity.LoginEvent{}, false
	}

	if u := after(msg, " for "); u != "" {
		ev.User, _, _ = strings.Cut(u, " ")
	}

	ev.IP = after(msg, " from ")

	return ev, ev.IP != ""
}

// after returns the text following the last occurrence of sep.
func after(s, sep string) string {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return ""
	}

	return strings.TrimSpace(s[i+len(sep):])
}

// quoted returns the first '...' substring.
func quoted(s string) string {
	_, rest, ok := strings.Cut(s, "'")
	if !ok {
		return ""
	}

	q, _, _ := strings.Cut(rest, "'")

	return q
}

// stripPort removes the ":port" dropbear appends to the address. IPv6
// addresses may be bracketed ("[fe80::1]:22") or bare ("fe80::1:22").
func stripPort(addr string) string {
	if strings.HasPrefix(addr, "[") {
		host, _, _ := strings.Cut(addr[1:], "]")

		return host
	}

	i := strings.LastIndexByte(addr, ':')
	if i < 0 {
		return addr
	}

	return addr[:i]
}
