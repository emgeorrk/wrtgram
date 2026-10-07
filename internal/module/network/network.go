// Package network reports the upstream link: /wan.
package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "network"

// Module implements module.Module.
type Module struct {
	wan usecase.WAN
}

// New creates the module.
func New(wan usecase.WAN) *Module { return &Module{wan: wan} }

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module: netifd is always there.
func (m *Module) Detect(context.Context) bool { return true }

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	return []module.Command{{Name: "wan", Description: "Upstream link: address, gateway, DNS", Handle: m.status}}
}

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier { return nil }

func (m *Module) status(ctx context.Context, _ module.Request) (module.Reply, error) {
	wan, err := m.wan.Status(ctx)
	if err != nil {
		return module.Reply{}, err
	}

	var b strings.Builder

	state := "❌ down"
	if wan.Up {
		state = "✅ up " + module.FormatDuration(wan.Uptime)
	}

	fmt.Fprintf(&b, "🌐 %s (%s, %s): %s\n", tgtext.B(wan.Interface), tgtext.Esc(wan.Device), tgtext.Esc(wan.Proto), state)

	if len(wan.IPv4) > 0 {
		fmt.Fprintf(&b, "Address: %s\n", tgtext.Code(strings.Join(wan.IPv4, ", ")))
	}

	if wan.Gateway != "" {
		fmt.Fprintf(&b, "Gateway: %s\n", tgtext.Code(wan.Gateway))
	}

	if len(wan.DNS) > 0 {
		fmt.Fprintf(&b, "DNS: %s\n", tgtext.Code(strings.Join(wan.DNS, ", ")))
	}

	return module.Reply{Text: strings.TrimRight(b.String(), "\n")}, nil
}
