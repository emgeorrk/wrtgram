// Package thermal exposes the hwmon sensors: /temp and overheat alerts.
package thermal

import (
	"context"
	"fmt"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	systemmod "github.com/emgeorrk/wrtgram/internal/module/system"
	"github.com/emgeorrk/wrtgram/internal/usecase/system"
	"github.com/emgeorrk/wrtgram/internal/usecase/thermal"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "thermal"

// Module implements module.Module.
type Module struct {
	svc     *system.Service
	watcher *thermal.Watcher // nil when alerts are disabled
}

// New creates the module.
func New(svc *system.Service, watcher *thermal.Watcher) *Module {
	return &Module{svc: svc, watcher: watcher}
}

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
func (m *Module) Notifiers() []module.Notifier {
	if m.watcher == nil {
		return nil
	}

	return []module.Notifier{notifier{m: m}}
}

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

type notifier struct {
	m *Module
}

func (notifier) Name() string { return Name }

func (n notifier) Run(ctx context.Context, notify module.Notify) error {
	return n.m.watcher.Run(ctx, func(ev thermal.Event) {
		notify.Message(ctx, entity.Message{Text: Render(ev)})
	})
}

// Render turns a thermal event into a message.
func Render(ev thermal.Event) string {
	line := strings.TrimPrefix(systemmod.TemperatureLine(ev.Temps), "🌡 ")
	line = strings.TrimSuffix(line, " ⚠️ hot")

	switch ev.Kind {
	case thermal.Overheat:
		return fmt.Sprintf("🔥 Router is overheating: %s. Check that the vents are clear and it is not in direct sunlight.", line)
	case thermal.StillHot:
		return "🔥 Router is still hot: " + line
	case thermal.Normal:
		return "✅ Router temperature is back to normal: " + line
	}

	return ""
}
