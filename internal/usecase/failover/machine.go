// Package failover keeps the default route on the best live VPN tunnel.
//
// The decision is a pure function (Step) over the previous state and the
// probe results, so every rule is unit-tested with a fake clock; Loop wraps
// it with pinging, route changes and persistence.
package failover

import (
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
)

// Rules is the static configuration of the controller.
type Rules struct {
	Tunnels     []string // priority order: the first is the primary
	Targets     []string // ping targets; one answer makes a tunnel healthy
	ServiceNets []string // networks always routed through a live tunnel
	Grace       time.Duration
	Interval    time.Duration
	NTPDirect   bool
}

// Primary is the first tunnel.
func (r Rules) Primary() string { return r.Tunnels[0] }

// Initial is the state assumed at start: routed via the primary, so the
// first tick only reports a deviation from the normal situation.
func Initial(r Rules, manualOff bool, now time.Time) entity.FailoverState {
	st := entity.FailoverState{Since: now, Mode: entity.FailoverPrimary, Dev: r.Primary(), ManualOff: manualOff}

	for _, t := range r.Tunnels {
		st.Tunnels = append(st.Tunnels, entity.TunnelHealth{Name: t, Healthy: true})
	}

	if manualOff {
		st.Mode, st.Dev = entity.FailoverDirect, ""
	}

	return st
}

// Step applies the probe results and decides where traffic should go.
//
//   - primary healthy → primary, immediately;
//   - the current tunnel just died → stay for Grace (anti-flap), then the
//     first healthy backup, else direct;
//   - manual off → direct, while the service networks still use a live tunnel.
func Step(prev entity.FailoverState, health map[string]bool, now time.Time, r Rules) (entity.FailoverState, []entity.FailoverEvent) {
	next := prev
	next.Tunnels = track(prev.Tunnels, r.Tunnels, health, now)
	next.CheckedAt = now

	want := decide(prev, next.Tunnels, health, now, r)

	// Service networks (the bot's own traffic) always take a live tunnel,
	// even while the default route waits out the grace on a dead one.
	next.ServiceDev = want
	if !health[want] {
		next.ServiceDev = firstHealthy(r.Tunnels, health)
	}

	var events []entity.FailoverEvent

	if want != prev.Dev {
		if !prev.ManualOff && !next.ManualOff {
			events = append(events, event(prev, want, now, r))
		}

		next.Dev, next.Since = want, now
	}

	switch {
	case want == "":
		next.Mode = entity.FailoverDirect
	case want == r.Primary():
		next.Mode = entity.FailoverPrimary
	default:
		next.Mode = entity.FailoverBackup
	}

	return next, events
}

func track(prev []entity.TunnelHealth, names []string, health map[string]bool, now time.Time) []entity.TunnelHealth {
	down := make(map[string]time.Time, len(prev))
	for _, t := range prev {
		down[t.Name] = t.DownSince
	}

	out := make([]entity.TunnelHealth, 0, len(names))

	for _, n := range names {
		th := entity.TunnelHealth{Name: n, Healthy: health[n]}
		if !th.Healthy {
			th.DownSince = down[n]
			if th.DownSince.IsZero() {
				th.DownSince = now
			}
		}

		out = append(out, th)
	}

	return out
}

func decide(prev entity.FailoverState, tunnels []entity.TunnelHealth, health map[string]bool, now time.Time, r Rules) string {
	if prev.ManualOff {
		return ""
	}

	if health[r.Primary()] {
		return r.Primary()
	}

	if prev.Dev != "" && !health[prev.Dev] && now.Sub(downSince(tunnels, prev.Dev)) < r.Grace {
		return prev.Dev
	}

	return firstHealthy(r.Tunnels, health)
}

func firstHealthy(names []string, health map[string]bool) string {
	for _, n := range names {
		if health[n] {
			return n
		}
	}

	return ""
}

func downSince(tunnels []entity.TunnelHealth, name string) time.Time {
	for _, t := range tunnels {
		if t.Name == name {
			return t.DownSince
		}
	}

	return time.Time{}
}

func event(prev entity.FailoverState, want string, now time.Time, r Rules) entity.FailoverEvent {
	ev := entity.FailoverEvent{From: prev.Dev, To: want, Downtime: now.Sub(prev.Since)}

	switch {
	case want == r.Primary():
		ev.Kind = entity.FailoverPrimaryBack
	case want == "":
		ev.Kind = entity.FailoverAllDown
	case prev.Dev == "":
		ev.Kind = entity.FailoverTunnelUp
	default:
		ev.Kind = entity.FailoverSwitched
	}

	return ev
}
