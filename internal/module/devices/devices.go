// Package devices lists LAN clients: /devices with inline pagination.
package devices

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase/devices"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "devices"

const (
	// DefaultPageSize is the number of devices per message.
	DefaultPageSize = 25
	pagePrefix      = "page:"
	unnamed         = "unnamed"
)

// Module implements module.Module.
type Module struct {
	svc      *devices.Service
	known    *devices.Known // nil when new-device notifications are off
	notify   module.Notify
	log      *slog.Logger
	detect   func(ctx context.Context) bool
	pageSize int
}

// New creates the module. detect reports whether any source exists; known
// enables the new-device notification.
func New(svc *devices.Service, known *devices.Known, detect func(context.Context) bool, pageSize int, log *slog.Logger) *Module {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	return &Module{svc: svc, known: known, detect: detect, pageSize: pageSize, log: log}
}

// OnDHCP handles a hotplug lease event: an address never seen before is
// announced. The first event seeds the set from the current leases so an
// install on a busy network stays quiet.
func (m *Module) OnDHCP(ctx context.Context, ev entity.DHCPEvent) {
	if m.known == nil || m.notify == nil || (ev.Action != "add" && ev.Action != "update") {
		return
	}

	if m.known.Len() == 0 {
		m.seed(ctx)
	}

	isNew, err := m.known.Observe(ev.MAC)
	if err != nil {
		m.log.Warn("known devices not saved", "err", err)
	}

	if !isNew {
		return
	}

	name := ev.Hostname
	if name == "" {
		name = unnamed
	}

	m.notify.Message(ctx, entity.Message{Text: fmt.Sprintf("🆕 New device on the network: %s\nIP: %s\nMAC: %s",
		tgtext.B(name), tgtext.Code(ev.IP), tgtext.Code(strings.ToLower(ev.MAC)))})
}

// seed records every current device as known.
func (m *Module) seed(ctx context.Context) {
	list, err := m.svc.List(ctx)
	if err != nil {
		return
	}

	macs := make([]string, 0, len(list))
	for _, d := range list {
		macs = append(macs, d.MAC)
	}

	if err := m.known.Seed(macs); err != nil {
		m.log.Warn("known devices not saved", "err", err)
	}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module.
func (m *Module) Detect(ctx context.Context) bool { return m.detect(ctx) }

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	return []module.Command{{
		Name: "devices", Description: "Connected devices", Handle: m.list, Callback: m.page,
	}}
}

// Notifiers implements module.Module: the new-device notifier only captures
// the Notify sink; events arrive through OnDHCP from the hotplug script.
func (m *Module) Notifiers() []module.Notifier {
	if m.known == nil {
		return nil
	}

	return []module.Notifier{sink{m: m}}
}

type sink struct {
	m *Module
}

func (sink) Name() string { return "new_device" }

func (s sink) Run(ctx context.Context, notify module.Notify) error {
	s.m.notify = notify

	<-ctx.Done()

	return nil
}

func (m *Module) list(ctx context.Context, _ module.Request) (module.Reply, error) {
	return m.render(ctx, 0, false)
}

func (m *Module) page(ctx context.Context, req module.Request) (module.Reply, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(req.Payload, pagePrefix))
	if err != nil {
		n = 0
	}

	return m.render(ctx, n, true)
}

func (m *Module) render(ctx context.Context, page int, edit bool) (module.Reply, error) {
	all, err := m.svc.List(ctx)
	if err != nil {
		return module.Reply{}, err
	}

	if len(all) == 0 {
		return module.Reply{Text: "No devices found."}, nil
	}

	pages := (len(all) + m.pageSize - 1) / m.pageSize
	if page < 0 || page >= pages {
		page = 0
	}

	start := page * m.pageSize
	end := min(start+m.pageSize, len(all))

	var b strings.Builder

	fmt.Fprintf(&b, "📱 %s", tgtext.B(fmt.Sprintf("%d devices", len(all))))

	if pages > 1 {
		fmt.Fprintf(&b, " (page %d/%d)", page+1, pages)
	}

	b.WriteString("\n")

	for _, d := range all[start:end] {
		b.WriteString(Line(d) + "\n")
	}

	reply := module.Reply{Text: strings.TrimRight(b.String(), "\n"), Edit: edit}

	if pages > 1 {
		var row []entity.Button

		if page > 0 {
			row = append(row, entity.Button{Text: "◀️", Data: fmt.Sprintf("devices:%s%d", pagePrefix, page-1)})
		}

		if page < pages-1 {
			row = append(row, entity.Button{Text: "▶️", Data: fmt.Sprintf("devices:%s%d", pagePrefix, page+1)})
		}

		reply.Keyboard = [][]entity.Button{row}
	}

	return reply, nil
}

// Line renders one device: "• iPhone — 192.168.1.231 · Wi-Fi 5 GHz −62 dBm".
func Line(d entity.Device) string {
	name := d.Hostname
	if name == "" {
		name = unnamed
	}

	var b strings.Builder

	fmt.Fprintf(&b, "• %s", tgtext.B(name))

	if d.IP != "" {
		fmt.Fprintf(&b, " — %s", tgtext.Code(d.IP))
	}

	switch {
	case d.Wireless && d.Band != "":
		fmt.Fprintf(&b, " · Wi-Fi %s", d.Band)
	case d.Wireless:
		b.WriteString(" · Wi-Fi")
	default:
		b.WriteString(" · wired or offline")
	}

	if d.Signal != 0 {
		fmt.Fprintf(&b, " %d dBm", d.Signal)
	}

	return b.String()
}
