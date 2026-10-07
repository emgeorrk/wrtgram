// Package devices lists LAN clients (/devices, /blocked) and announces new
// ones with buttons to pin their address or block them.
package devices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase/devices"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "devices"

var errUnknownAction = errors.New("unknown device action")

const (
	// DefaultPageSize is the number of devices per message.
	DefaultPageSize = 25
	pagePrefix      = "page:"
	unnamed         = "unnamed"
	cmdDevice       = "device" // hidden: callbacks of the new-device card
	actRemember     = "remember"
	actBlock        = "block"
	actUnblock      = "unblock"
	fromList        = "list"
)

// Module implements module.Module.
type Module struct {
	svc      *devices.Service
	manager  *devices.Manager
	known    *devices.Known // nil when new-device notifications are off
	notify   module.Notify
	log      *slog.Logger
	detect   func(ctx context.Context) bool
	names    map[string]string // MAC → hostname from hotplug events (for the card buttons)
	mu       sync.Mutex
	pageSize int
}

// New creates the module. detect reports whether any source exists; known
// enables the new-device notification.
func New(svc *devices.Service, manager *devices.Manager, known *devices.Known, detect func(context.Context) bool,
	pageSize int, log *slog.Logger,
) *Module {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	return &Module{svc: svc, manager: manager, known: known, detect: detect, pageSize: pageSize, log: log, names: map[string]string{}}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module.
func (m *Module) Detect(ctx context.Context) bool { return m.detect(ctx) }

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	return []module.Command{
		{Name: "devices", Description: "Connected devices", Handle: m.list, Callback: m.page},
		{Name: "blocked", Description: "Blocked devices", Handle: m.blocked},
		{Name: cmdDevice, Hidden: true, Handle: m.unknown, Callback: m.action},
	}
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

// OnDHCP handles a hotplug lease event: an address never seen before is
// announced with action buttons. On a fresh install the first event seeds
// the set from the current leases so a busy network stays quiet; an emptied
// known_macs file instead yields a card for every device.
func (m *Module) OnDHCP(ctx context.Context, ev entity.DHCPEvent) {
	if m.known == nil || m.notify == nil || (ev.Action != "add" && ev.Action != "update") {
		return
	}

	if m.known.Fresh() {
		m.seed(ctx)
	}

	isNew, err := m.known.Observe(ev.MAC)
	if err != nil {
		m.log.Warn("known devices not saved", "err", err)
	}

	if !isNew {
		return
	}

	mac := strings.ToLower(ev.MAC)

	if ev.Hostname != "" {
		m.mu.Lock()
		m.names[mac] = ev.Hostname
		m.mu.Unlock()
	}

	m.notify.Message(ctx, entity.Message{
		Text:     card(mac, ev.IP, ev.Hostname, ""),
		Keyboard: m.cardKeyboard(ctx, mac, ev.IP),
	})
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

// card renders the new-device message; result is an optional status line.
func card(mac, ip, name, result string) string {
	if name == "" {
		name = unnamed
	}

	text := fmt.Sprintf("🆕 New device on the network: %s\nIP: %s\nMAC: %s", tgtext.B(name), tgtext.Code(ip), tgtext.Code(mac))
	if result != "" {
		text += "\n\n" + result
	}

	return text
}

// cardKeyboard offers the actions that still make sense for mac.
func (m *Module) cardKeyboard(ctx context.Context, mac, ip string) [][]entity.Button {
	static, blocked := m.manager.Flags(ctx)

	var row []entity.Button

	if !static[mac] && ip != "" {
		row = append(row, entity.Button{Text: "📌 Remember IP", Data: strings.Join([]string{cmdDevice, actRemember, mac, ip}, ":")})
	}

	if blocked[mac] {
		row = append(row, entity.Button{Text: "✅ Unblock", Data: strings.Join([]string{cmdDevice, actUnblock, mac}, ":")})
	} else {
		row = append(row, entity.Button{Text: "⛔ Block", Data: strings.Join([]string{cmdDevice, actBlock, mac}, ":")})
	}

	return [][]entity.Button{row}
}

func (m *Module) unknown(context.Context, module.Request) (module.Reply, error) {
	return module.Reply{Text: "Use the buttons under a device notification, or /devices and /blocked."}, nil
}

// action serves the card buttons: payload "<action>:<mac>[:<ip>|:list]".
func (m *Module) action(ctx context.Context, req module.Request) (module.Reply, error) {
	parts := strings.Split(req.Payload, ":")
	if len(parts) < 1+macParts {
		return module.Reply{Toast: "Bad request"}, nil
	}

	act := parts[0]
	mac := strings.Join(parts[1:1+macParts], ":")
	extra := strings.Join(parts[1+macParts:], ":")

	result, err := m.perform(ctx, act, mac, extra)
	if err != nil {
		return module.Reply{}, err
	}

	if extra == fromList {
		reply, err := m.blocked(ctx, req)
		reply.Edit, reply.Toast = true, result

		return reply, err
	}

	ip, name := m.lookup(ctx, mac)
	if act == actRemember {
		ip = extra
	}

	if name == "" {
		name = m.eventName(mac)
	}

	return module.Reply{Text: card(mac, ip, name, result), Keyboard: m.cardKeyboard(ctx, mac, ip), Edit: true, Toast: result}, nil
}

// perform executes one card action and describes the outcome.
func (m *Module) perform(ctx context.Context, act, mac, extra string) (string, error) {
	switch act {
	case actRemember:
		if err := m.manager.Remember(ctx, mac, extra, m.nameOf(ctx, mac)); err != nil {
			return "", err
		}

		result := "📌 Static lease " + tgtext.Code(extra) + " saved."
		if m.nameOf(ctx, mac) == "" {
			result += " The name will show up once the device reports one."
		}

		return result, nil
	case actBlock:
		return "⛔ Blocked: all traffic from this device is rejected.", m.manager.Block(ctx, mac)
	case actUnblock:
		return "✅ Unblocked.", m.manager.Unblock(ctx, mac)
	}

	return "", fmt.Errorf("%w: %q", errUnknownAction, act)
}

const macParts = 6

func (m *Module) lookup(ctx context.Context, mac string) (ip, name string) {
	list, err := m.svc.List(ctx)
	if err != nil {
		return "", ""
	}

	for _, d := range list {
		if d.MAC == mac {
			return d.IP, d.Hostname
		}
	}

	return "", ""
}

// nameOf prefers the lease/hint name and falls back to the hotplug event's.
func (m *Module) nameOf(ctx context.Context, mac string) string {
	if _, name := m.lookup(ctx, mac); name != "" {
		return name
	}

	return m.eventName(mac)
}

func (m *Module) eventName(mac string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.names[mac]
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

// all returns the device list with the static/blocked flags applied.
func (m *Module) all(ctx context.Context) ([]entity.Device, error) {
	list, err := m.svc.List(ctx)
	if err != nil {
		return nil, err
	}

	static, blocked := m.manager.Flags(ctx)
	for i := range list {
		list[i].Static, list[i].Blocked = static[list[i].MAC], blocked[list[i].MAC]
	}

	return list, nil
}

func (m *Module) render(ctx context.Context, page int, edit bool) (module.Reply, error) {
	all, err := m.all(ctx)
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

// blocked lists the blocked devices with an Unblock button each.
func (m *Module) blocked(ctx context.Context, _ module.Request) (module.Reply, error) {
	_, blockedMACs := m.manager.Flags(ctx)
	if len(blockedMACs) == 0 {
		return module.Reply{Text: "No blocked devices. Use the ⛔ Block button under a new-device notification."}, nil
	}

	names := map[string]string{}

	if list, err := m.svc.List(ctx); err == nil {
		for _, d := range list {
			names[d.MAC] = d.Hostname
		}
	}

	var (
		b    strings.Builder
		rows [][]entity.Button
	)

	b.WriteString("⛔ " + tgtext.B("Blocked devices") + "\n")

	for mac := range blockedMACs {
		name := names[mac]
		if name == "" {
			name = unnamed
		}

		fmt.Fprintf(&b, "• %s — %s\n", tgtext.B(name), tgtext.Code(mac))

		rows = append(rows, []entity.Button{{Text: "✅ Unblock " + name, Data: strings.Join([]string{cmdDevice, actUnblock, mac, fromList}, ":")}})
	}

	return module.Reply{Text: strings.TrimRight(b.String(), "\n"), Keyboard: rows}, nil
}

// Line renders one device: "• 📌 iPhone — 192.168.1.231 · Wi-Fi 5 GHz −62 dBm".
func Line(d entity.Device) string {
	name := d.Hostname
	if name == "" {
		name = unnamed
	}

	var b strings.Builder

	b.WriteString("• ")

	if d.Blocked {
		b.WriteString("⛔ ")
	}

	if d.Static {
		b.WriteString("📌 ")
	}

	b.WriteString(tgtext.B(name))

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
