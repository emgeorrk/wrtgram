// Package services lists procd services and restarts whitelisted ones.
package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase/services"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "services"

// Module implements module.Module.
type Module struct {
	svc *services.Service
}

// New creates the module.
func New(svc *services.Service) *Module { return &Module{svc: svc} }

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module: procd is always there.
func (m *Module) Detect(context.Context) bool { return true }

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	return []module.Command{
		{Name: "services", Description: "Running services", Handle: m.list},
		{Name: "restart", Description: "Restart a service: /restart <name>", Handle: m.restart, Confirm: true},
	}
}

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier { return nil }

func (m *Module) list(ctx context.Context, _ module.Request) (module.Reply, error) {
	list, err := m.svc.List(ctx)
	if err != nil {
		return module.Reply{}, err
	}

	var b strings.Builder

	b.WriteString("⚙️ " + tgtext.B("Services") + "\n")

	for _, s := range list {
		mark := "✅"

		switch {
		case s.Instances == 0:
			mark = "▫️"
		case s.Running < s.Instances:
			mark = "❌"
		}

		fmt.Fprintf(&b, "%s %s", mark, tgtext.Esc(s.Name))

		if s.Instances > 1 {
			fmt.Fprintf(&b, " (%d/%d)", s.Running, s.Instances)
		}

		b.WriteString("\n")
	}

	if wl := m.svc.Whitelist(); len(wl) > 0 {
		fmt.Fprintf(&b, "\nRestartable: %s", tgtext.Esc(strings.Join(wl, ", ")))
	}

	return module.Reply{Text: strings.TrimRight(b.String(), "\n")}, nil
}

// restart is a Confirm command: the first call validates and asks, the
// confirmed callback (with the original arguments) performs it.
func (m *Module) restart(ctx context.Context, req module.Request) (module.Reply, error) {
	if len(req.Args) == 0 {
		return module.Reply{Text: "Usage: /restart <service>\nRestartable: " + tgtext.Esc(strings.Join(m.svc.Whitelist(), ", "))}, nil
	}

	name := req.Args[0]

	if !req.FromCallback {
		if !m.svc.Allowed(name) {
			return module.Reply{Text: fmt.Sprintf("%s is not in the restart whitelist (list service '%s' in /etc/config/wrtgram).",
				tgtext.Code(name), tgtext.Esc(name))}, nil
		}

		return module.Reply{Text: fmt.Sprintf("Restart %s?", tgtext.Code(name))}, nil
	}

	if err := m.svc.Restart(ctx, name); err != nil {
		return module.Reply{}, err
	}

	return module.Reply{Text: fmt.Sprintf("🔄 %s restarted.", tgtext.Code(name)), Edit: true}, nil
}
