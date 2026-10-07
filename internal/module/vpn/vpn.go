// Package vpn shows tunnel state and switches the VPN: /vpn, /vpn_on, /vpn_off.
package vpn

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase"
	"github.com/emgeorrk/wrtgram/internal/usecase/vpn"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "vpn"

const (
	handshakeFresh = 3 * time.Minute
	settle         = 2 * time.Second
	cbOn           = "on"
	cbOff          = "off"
)

// Module implements module.Module.
type Module struct {
	svc    *vpn.Service
	clock  usecase.Clock
	detect func(ctx context.Context) bool
}

// New creates the module.
func New(svc *vpn.Service, clock usecase.Clock, detect func(context.Context) bool) *Module {
	return &Module{svc: svc, clock: clock, detect: detect}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module.
func (m *Module) Detect(ctx context.Context) bool { return m.detect(ctx) }

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	return []module.Command{
		{Name: "vpn", Description: "VPN tunnels and failover state", Handle: m.status, Callback: m.toggle},
		{Name: "vpn_off", Description: "Route all traffic directly, without VPN", Handle: m.off},
		{Name: "vpn_on", Description: "Route traffic through the VPN again", Handle: m.on},
	}
}

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier { return nil }

// StatusSection renders the VPN line for /status.
func (m *Module) StatusSection(ctx context.Context) string {
	tunnels, err := m.svc.List(ctx)
	if err != nil {
		return ""
	}

	return m.summary(tunnels)
}

func (m *Module) status(ctx context.Context, _ module.Request) (module.Reply, error) {
	return m.render(ctx, false)
}

func (m *Module) toggle(ctx context.Context, req module.Request) (module.Reply, error) {
	switch req.Payload {
	case cbOn, cbOff:
		if err := m.svc.SetEnabled(ctx, req.Payload == cbOn); err != nil {
			return module.Reply{}, err
		}

		time.Sleep(settle)
	}

	reply, err := m.render(ctx, true)
	reply.Toast = "VPN " + req.Payload

	return reply, err
}

func (m *Module) off(ctx context.Context, _ module.Request) (module.Reply, error) {
	if err := m.svc.SetEnabled(ctx, false); err != nil {
		return module.Reply{}, err
	}

	time.Sleep(settle)

	reply, err := m.render(ctx, false)
	reply.Text = "⏸ VPN switched off. Traffic goes directly through the ISP; the bot keeps using a tunnel.\n\n" + reply.Text

	return reply, err
}

func (m *Module) on(ctx context.Context, _ module.Request) (module.Reply, error) {
	if err := m.svc.SetEnabled(ctx, true); err != nil {
		return module.Reply{}, err
	}

	time.Sleep(settle)

	reply, err := m.render(ctx, false)
	reply.Text = "▶️ VPN switched on.\n\n" + reply.Text

	return reply, err
}

func (m *Module) render(ctx context.Context, edit bool) (module.Reply, error) {
	tunnels, err := m.svc.List(ctx)
	if err != nil {
		return module.Reply{}, err
	}

	var b strings.Builder

	b.WriteString(m.summary(tunnels) + "\n")

	for _, t := range tunnels {
		b.WriteString(m.tunnelLine(t) + "\n")
	}

	reply := module.Reply{Text: strings.TrimRight(b.String(), "\n"), Edit: edit}

	if fo := m.svc.Failover(); fo != nil {
		if fo.State().ManualOff {
			reply.Keyboard = [][]entity.Button{{{Text: "▶️ VPN on", Data: "vpn:" + cbOn}}}
		} else {
			reply.Keyboard = [][]entity.Button{{{Text: "⏸ VPN off", Data: "vpn:" + cbOff}}}
		}
	}

	return reply, nil
}

// summary is the headline: failover mode when available, else tunnel counts.
func (m *Module) summary(tunnels []entity.Tunnel) string {
	if fo := m.svc.Failover(); fo != nil {
		st := fo.State()

		switch {
		case st.ManualOff:
			return "🛡 VPN: ⏸ switched off manually, traffic goes direct. Turn on: /vpn_on"
		case st.Mode == entity.FailoverPrimary:
			return fmt.Sprintf("🛡 VPN: primary %s ✅ (for %s)", tgtext.Code(st.Dev), module.FormatDuration(m.clock.Now().Sub(st.Since)))
		case st.Mode == entity.FailoverBackup:
			return fmt.Sprintf("🛡 VPN: ⚠️ backup %s (for %s)", tgtext.Code(st.Dev), module.FormatDuration(m.clock.Now().Sub(st.Since)))
		}

		return fmt.Sprintf("🛡 VPN: 🚨 all tunnels down, traffic goes direct (for %s)", module.FormatDuration(m.clock.Now().Sub(st.Since)))
	}

	up := 0

	for _, t := range tunnels {
		if t.Handshaked(m.clock.Now(), handshakeFresh) {
			up++
		}
	}

	return fmt.Sprintf("🛡 VPN: %d of %d tunnels active", up, len(tunnels))
}

func (m *Module) tunnelLine(t entity.Tunnel) string {
	name := tgtext.Code(t.Ref.Name)

	if !t.Up {
		return fmt.Sprintf("%s: ❌ down", name)
	}

	var hs time.Time

	var rx, tx uint64

	for _, p := range t.Peers {
		if p.LastHandshake.After(hs) {
			hs = p.LastHandshake
		}

		rx += p.RxBytes
		tx += p.TxBytes
	}

	if hs.IsZero() {
		return fmt.Sprintf("%s: ⚠️ up, no handshake yet", name)
	}

	age := m.clock.Now().Sub(hs)
	mark := "✅"

	if age > handshakeFresh {
		mark = "⚠️"
	}

	return fmt.Sprintf("%s: %s handshake %s ago, ↓%s ↑%s", name, mark, module.FormatDuration(age),
		module.FormatBytes(rx), module.FormatBytes(tx))
}
