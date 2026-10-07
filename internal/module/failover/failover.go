// Package failover exposes the controller: /failover and the switch notifications.
package failover

import (
	"context"
	"fmt"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase"
	"github.com/emgeorrk/wrtgram/internal/usecase/failover"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "failover"

// Module implements module.Module.
type Module struct {
	loop   *failover.Loop
	clock  usecase.Clock
	notify bool
}

// New creates the module; notify enables the switch messages.
func New(loop *failover.Loop, clock usecase.Clock, notify bool) *Module {
	return &Module{loop: loop, clock: clock, notify: notify}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module: the app builds the loop only when the
// module is configured and no legacy controller runs.
func (m *Module) Detect(context.Context) bool { return m.loop != nil }

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	return []module.Command{{Name: Name, Description: "Failover details: tunnels, timers, routes", Handle: m.details}}
}

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier { return []module.Notifier{notifier{m: m}} }

func (m *Module) details(context.Context, module.Request) (module.Reply, error) {
	st := m.loop.State()
	r := m.loop.Rules()
	now := m.clock.Now()

	var b strings.Builder

	fmt.Fprintf(&b, "🛡 %s\n", tgtext.B("VPN failover"))
	fmt.Fprintf(&b, "Mode: %s", st.Mode)

	if st.Dev != "" {
		fmt.Fprintf(&b, " via %s", tgtext.Code(st.Dev))
	}

	fmt.Fprintf(&b, " for %s\n", module.FormatDuration(now.Sub(st.Since)))

	if st.ManualOff {
		b.WriteString("Manual off: yes (/vpn_on to resume)\n")
	}

	for i, t := range st.Tunnels {
		role := "backup"
		if i == 0 {
			role = "primary"
		}

		if t.Healthy {
			fmt.Fprintf(&b, "• %s (%s): ✅ healthy\n", tgtext.Code(t.Name), role)
		} else {
			fmt.Fprintf(&b, "• %s (%s): ❌ down for %s\n", tgtext.Code(t.Name), role, module.FormatDuration(now.Sub(t.DownSince)))
		}
	}

	fmt.Fprintf(&b, "Service networks via: %s\n", orNone(st.ServiceDev))
	fmt.Fprintf(&b, "Check every %s, grace %s, last check %s ago",
		module.FormatDuration(r.Interval), module.FormatDuration(r.Grace), module.FormatDuration(now.Sub(st.CheckedAt)))

	return module.Reply{Text: b.String()}, nil
}

func orNone(dev string) string {
	if dev == "" {
		return "none"
	}

	return tgtext.Code(dev)
}

type notifier struct {
	m *Module
}

func (notifier) Name() string { return "failover" }

func (n notifier) Run(ctx context.Context, notify module.NotifyFunc) error {
	return n.m.loop.Run(ctx, func(ev entity.FailoverEvent) {
		if n.m.notify {
			notify(ctx, entity.Message{Text: Render(ev)})
		}
	})
}

// Render turns a routing change into a message.
func Render(ev entity.FailoverEvent) string {
	switch ev.Kind {
	case entity.FailoverPrimaryBack:
		return fmt.Sprintf("✅ VPN: primary tunnel %s is back, traffic returned to it (was away %s)",
			tgtext.Code(ev.To), module.FormatDuration(ev.Downtime))
	case entity.FailoverSwitched:
		return fmt.Sprintf("⚠️ VPN: %s has been down for %s — switched to backup %s",
			tgtext.Code(ev.From), module.FormatDuration(ev.Downtime), tgtext.Code(ev.To))
	case entity.FailoverAllDown:
		return "🚨 VPN: all tunnels are down — traffic goes directly through the ISP, without VPN"
	case entity.FailoverTunnelUp:
		return fmt.Sprintf("✅ VPN: tunnel %s is up — traffic goes through it again (was direct for %s)",
			tgtext.Code(ev.To), module.FormatDuration(ev.Downtime))
	}

	return ""
}
