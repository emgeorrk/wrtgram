// Package system is the always-on module: /status, /reboot and the boot notification.
package system

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase/system"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "system"

const (
	hotCPU     = 85.0
	bootWindow = 5 * time.Minute
	rebootWait = 2 * time.Second
)

// Module implements module.Module.
type Module struct {
	svc        *system.Service
	log        *slog.Logger
	extra      []StatusSection
	bootNotify bool
}

// StatusSection lets other modules append lines to /status (e.g. the VPN state).
type StatusSection func(ctx context.Context) string

// New creates the module. bootNotify enables the "router started" message.
func New(svc *system.Service, log *slog.Logger, bootNotify bool) *Module {
	return &Module{svc: svc, log: log, bootNotify: bootNotify}
}

// AddStatusSection registers extra /status content.
func (m *Module) AddStatusSection(s StatusSection) { m.extra = append(m.extra, s) }

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module: the system module always applies.
func (m *Module) Detect(context.Context) bool { return true }

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	return []module.Command{
		{Name: "status", Description: "Router status: uptime, load, memory, temperature, WAN", Handle: m.status},
		{Name: "reboot", Description: "Reboot the router", Handle: m.reboot, Confirm: true},
	}
}

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier {
	if !m.bootNotify {
		return nil
	}

	return []module.Notifier{bootNotifier{svc: m.svc}}
}

func (m *Module) status(ctx context.Context, _ module.Request) (module.Reply, error) {
	snap, err := m.svc.Snapshot(ctx)
	if err != nil {
		return module.Reply{}, err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "📡 %s\n", tgtext.B(snap.Board.Model))
	fmt.Fprintf(&b, "%s · %s\n", tgtext.Esc(snap.Board.Hostname), tgtext.Esc(snap.Board.Release))
	fmt.Fprintf(&b, "Uptime: %s, load: %.2f %.2f %.2f\n", module.FormatDuration(snap.Info.Uptime),
		snap.Info.Load[0], snap.Info.Load[1], snap.Info.Load[2])
	fmt.Fprintf(&b, "Memory: %s free of %s\n", module.FormatBytes(snap.Info.MemAvail), module.FormatBytes(snap.Info.MemTotal))

	if snap.Info.FlashTotal > 0 {
		fmt.Fprintf(&b, "Flash: %s free of %s\n", module.FormatBytes(snap.Info.FlashFree), module.FormatBytes(snap.Info.FlashTotal))
	}

	if snap.TempsErr == nil {
		b.WriteString(TemperatureLine(snap.Temps) + "\n")
	}

	b.WriteString(wanLine(snap.WAN, snap.WANErr) + "\n")

	for _, section := range m.extra {
		if s := section(ctx); s != "" {
			b.WriteString("\n" + s + "\n")
		}
	}

	return module.Reply{Text: strings.TrimRight(b.String(), "\n")}, nil
}

// TemperatureLine renders "🌡 CPU 66°C, Wi-Fi 60°C" from hwmon readings.
func TemperatureLine(temps []entity.Temperature) string {
	if len(temps) == 0 {
		return ""
	}

	parts := make([]string, 0, len(temps))
	hot := false

	for _, t := range temps {
		parts = append(parts, fmt.Sprintf("%s %.0f°C", tgtext.Esc(SensorName(t)), t.Celsius))

		if t.Celsius >= hotCPU {
			hot = true
		}
	}

	line := "🌡 " + strings.Join(parts, ", ")
	if hot {
		line += " ⚠️ hot"
	}

	return line
}

// SensorName maps hwmon names to something a human recognizes.
func SensorName(t entity.Temperature) string {
	name := strings.ToLower(t.Sensor)

	switch {
	case strings.Contains(name, "cpu") || strings.Contains(name, "soc"):
		return "CPU"
	case strings.HasPrefix(name, "mt76") || strings.HasPrefix(name, "mt79") || strings.Contains(name, "phy") ||
		strings.Contains(name, "wifi") || strings.Contains(name, "ath1") || strings.Contains(name, "iwl"):
		return wifiName(name)
	}

	if t.Label != "" {
		return t.Label
	}

	return t.Sensor
}

func wifiName(name string) string {
	switch {
	case strings.HasSuffix(name, "phy0"):
		return "Wi-Fi 0"
	case strings.HasSuffix(name, "phy1"):
		return "Wi-Fi 1"
	case strings.HasSuffix(name, "phy2"):
		return "Wi-Fi 2"
	}

	return "Wi-Fi"
}

func wanLine(wan entity.WANStatus, err error) string {
	if err != nil {
		return "WAN: ❓ unknown"
	}

	if !wan.Up {
		return "WAN: ❌ down"
	}

	line := fmt.Sprintf("WAN: ✅ up %s", module.FormatDuration(wan.Uptime))
	if len(wan.IPv4) > 0 {
		line += " · " + tgtext.Code(wan.IPv4[0])
	}

	return line
}

func (m *Module) reboot(ctx context.Context, req module.Request) (module.Reply, error) {
	if !req.FromCallback {
		return module.Reply{Text: "Reboot the router?"}, nil
	}

	m.log.Warn("reboot requested via Telegram", "chat", req.ChatID)

	go func() {
		time.Sleep(rebootWait) // let the reply leave first

		if err := m.svc.Reboot(context.WithoutCancel(ctx)); err != nil {
			m.log.Error("reboot failed", "err", err)
		}
	}()

	return module.Reply{Text: "🔄 Rebooting, back in a couple of minutes.", Edit: true}, nil
}

type bootNotifier struct {
	svc *system.Service
}

func (bootNotifier) Name() string { return "boot" }

// Run sends one message when the daemon starts shortly after boot, then
// idles. A procd respawn later in the uptime stays silent.
func (n bootNotifier) Run(ctx context.Context, notify module.Notify) error {
	info, err := n.svc.Info(ctx)
	if err != nil {
		return err
	}

	if info.Uptime < bootWindow {
		host := "router"
		if board, err := n.svc.Board(ctx); err == nil && board.Hostname != "" {
			host = board.Hostname
		}

		notify.Message(ctx, entity.Message{Text: fmt.Sprintf("✅ Router %s started (uptime %s)",
			tgtext.B(host), module.FormatDuration(info.Uptime))})
	}

	<-ctx.Done()

	return nil
}
