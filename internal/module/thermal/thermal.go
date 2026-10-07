// Package thermal exposes the hwmon sensors: /temp now, overheat alerts in M3.
package thermal

import (
	"context"
	"fmt"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/module"
	systemmod "github.com/emgeorrk/wrtgram/internal/module/system"
	"github.com/emgeorrk/wrtgram/internal/usecase/system"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "thermal"

// Module implements module.Module.
type Module struct {
	svc *system.Service
}

// New creates the module.
func New(svc *system.Service) *Module { return &Module{svc: svc} }

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module: at least one hwmon temperature sensor.
func (m *Module) Detect(context.Context) bool {
	_, err := m.svc.Temperatures()

	return err == nil
}

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	return []module.Command{{Name: "temp", Description: "Temperature sensors", Handle: m.temp}}
}

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier { return nil }

func (m *Module) temp(context.Context, module.Request) (module.Reply, error) {
	temps, err := m.svc.Temperatures()
	if err != nil {
		return module.Reply{}, err
	}

	var b strings.Builder

	b.WriteString(systemmod.TemperatureLine(temps) + "\n")

	for _, t := range temps {
		label := t.Sensor
		if t.Label != "" {
			label += "/" + t.Label
		}

		fmt.Fprintf(&b, "• %s: %.1f°C\n", tgtext.Code(label), t.Celsius)
	}

	return module.Reply{Text: strings.TrimRight(b.String(), "\n")}, nil
}
