// Package iproute drives `ip` and `ping` for the failover controller.
package iproute

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// Routing constants shared with the documentation: the two /1 routes cover
// the whole IPv4 space with a better prefix than the WAN default route.
const (
	HalfLow  = "0.0.0.0/1"
	HalfHigh = "128.0.0.0/1"

	NTPRulePref = "29990"
	NTPTable    = "123"
	ntpPort     = "123"
	pingTimeout = 8 * time.Second
	sysntpd     = "/etc/init.d/sysntpd"
)

var (
	errRoute = errors.New("ip route")
	errRule  = errors.New("ip rule")
)

// Router implements usecase.Router.
type Router struct {
	run        usecase.Runner
	log        *slog.Logger
	ntpApplied string
	mu         sync.Mutex
}

// NewRouter creates the adapter.
func NewRouter(run usecase.Runner, log *slog.Logger) *Router { return &Router{run: run, log: log} }

// DefaultDev implements usecase.Router.
func (r *Router) DefaultDev(ctx context.Context) (string, error) {
	raw, err := r.run.Run(ctx, "ip", "route", "show", HalfLow)
	if err != nil {
		return "", fmt.Errorf("%w: show: %w", errRoute, err)
	}

	return DevOf(string(raw)), nil
}

// DevOf extracts the device from an `ip route show` line.
func DevOf(line string) string {
	f := strings.Fields(line)
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "dev" {
			return f[i+1]
		}
	}

	return ""
}

// SetDefault implements usecase.Router.
func (r *Router) SetDefault(ctx context.Context, dev string) error {
	for _, net := range []string{HalfLow, HalfHigh} {
		if _, err := r.run.Run(ctx, "ip", "route", "replace", net, "dev", dev); err != nil {
			return fmt.Errorf("%w: replace %s via %s: %w", errRoute, net, dev, err)
		}
	}

	return nil
}

// ClearDefault implements usecase.Router; a missing route is not an error.
func (r *Router) ClearDefault(ctx context.Context) error {
	for _, net := range []string{HalfLow, HalfHigh} {
		r.del(ctx, net)
	}

	return nil
}

// del removes a route; "no such process" for an absent route is expected.
func (r *Router) del(ctx context.Context, net string) {
	if _, err := r.run.Run(ctx, "ip", "route", "del", net); err != nil {
		r.log.Debug("ip route del", "net", net, "err", err)
	}
}

// ReplaceNets implements usecase.Router.
func (r *Router) ReplaceNets(ctx context.Context, nets []string, dev string) error {
	for _, net := range nets {
		if _, err := r.run.Run(ctx, "ip", "route", "replace", net, "dev", dev); err != nil {
			return fmt.Errorf("%w: replace %s via %s: %w", errRoute, net, dev, err)
		}
	}

	return nil
}

// DeleteNets implements usecase.Router.
func (r *Router) DeleteNets(ctx context.Context, nets []string) error {
	for _, net := range nets {
		r.del(ctx, net)
	}

	return nil
}

// EnsureNTPDirect implements usecase.Router: the rule sends the router's
// own NTP packets to table 123, whose default route follows the WAN
// gateway. sysntpd is restarted when the gateway changes so it polls at
// once instead of waiting out its back-off.
func (r *Router) EnsureNTPDirect(ctx context.Context, gw, dev string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	raw, err := r.run.Run(ctx, "ip", "rule", "show", "pref", NTPRulePref)
	if err != nil {
		return false, fmt.Errorf("%w: show: %w", errRule, err)
	}

	if strings.TrimSpace(string(raw)) == "" {
		_, err := r.run.Run(ctx, "ip", "rule", "add", "pref", NTPRulePref, "iif", "lo", "ipproto", "udp", "dport", ntpPort, "lookup", NTPTable)
		if err != nil {
			return false, fmt.Errorf("%w: add (needs ip-full): %w", errRule, err)
		}
	}

	key := gw + " " + dev
	if key == r.ntpApplied {
		return false, nil
	}

	args := []string{"route", "replace", "default"}
	if gw != "" {
		args = append(args, "via", gw)
	}

	args = append(args, "dev", dev, "table", NTPTable)

	if _, err := r.run.Run(ctx, "ip", args...); err != nil {
		return false, fmt.Errorf("%w: table %s: %w", errRoute, NTPTable, err)
	}

	r.ntpApplied = key

	if _, err := r.run.Run(ctx, sysntpd, "restart"); err != nil {
		r.log.Debug("sysntpd restart", "err", err)
	}

	return true, nil
}

// Pinger implements usecase.Pinger with busybox ping.
type Pinger struct {
	run usecase.Runner
}

// NewPinger creates the adapter.
func NewPinger(run usecase.Runner) *Pinger { return &Pinger{run: run} }

// Ping implements usecase.Pinger: two probes, two seconds each, bound to dev.
func (p *Pinger) Ping(ctx context.Context, dev, target string) bool {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	_, err := p.run.Run(ctx, "ping", "-q", "-c", "2", "-W", "2", "-I", dev, target)

	return err == nil
}
