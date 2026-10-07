// Package devices lists LAN clients: /devices with inline pagination.
package devices

import (
	"context"
	"fmt"
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
)

// Module implements module.Module.
type Module struct {
	svc      *devices.Service
	detect   func(ctx context.Context) bool
	pageSize int
}

// New creates the module. detect reports whether any source exists.
func New(svc *devices.Service, detect func(context.Context) bool, pageSize int) *Module {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	return &Module{svc: svc, detect: detect, pageSize: pageSize}
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

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier { return nil }

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
		name = "unnamed"
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
