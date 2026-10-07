package devices

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// UCI packages and section types the manager touches.
const (
	pkgDHCP      = "dhcp"
	pkgFirewall  = "firewall"
	typeHost     = "host"
	typeRule     = "rule"
	typeZone     = "zone"
	rulePrefix   = "wrtgram_block_"
	dnsmasqInit  = "/etc/init.d/dnsmasq"
	firewallInit = "/etc/init.d/firewall"
	lanZone      = "lan"
	targetReject = "REJECT"
	protoAll     = "all"
	optEnabled   = "enabled"
)

var (
	errBadMAC = errors.New("invalid MAC address")
	errBadIP  = errors.New("invalid IP address")
)

// Manager pins addresses and blocks devices through UCI.
type Manager struct {
	uci usecase.UCI
	run usecase.Runner
}

// NewManager creates the manager.
func NewManager(uci usecase.UCI, run usecase.Runner) *Manager {
	return &Manager{uci: uci, run: run}
}

// Flags reports which MACs have a static lease and which are blocked.
func (m *Manager) Flags(ctx context.Context) (static, blocked map[string]bool) {
	static, blocked = map[string]bool{}, map[string]bool{}

	if pkg, err := m.uci.Get(ctx, pkgDHCP); err == nil {
		for _, h := range pkg.OfType(typeHost) {
			for _, mac := range h.List("mac") {
				static[strings.ToLower(mac)] = true
			}
		}
	}

	if pkg, err := m.uci.Get(ctx, pkgFirewall); err == nil {
		for _, r := range pkg.OfType(typeRule) {
			if strings.HasPrefix(r.Name, rulePrefix) && r.Bool(optEnabled, true) {
				blocked[strings.ToLower(r.Opt("src_mac", ""))] = true
			}
		}
	}

	return static, blocked
}

// Remember gives mac a static DHCP lease for ip (an existing lease for the
// same MAC is updated).
func (m *Manager) Remember(ctx context.Context, mac, ip, name string) error {
	mac, ok := normMAC(mac)
	if !ok {
		return fmt.Errorf("%w: %q", errBadMAC, mac)
	}

	if strings.TrimSpace(ip) == "" || strings.ContainsAny(ip, " '\"") {
		return fmt.Errorf("%w: %q", errBadIP, ip)
	}

	section, err := m.hostSection(ctx, mac)
	if err != nil {
		return err
	}

	values := map[string]any{"mac": mac, "ip": ip}
	if name = hostName(name, mac); name != "" {
		values["name"] = name
	}

	if err := m.uci.Set(ctx, pkgDHCP, section, values); err != nil {
		return err
	}

	if err := m.uci.Commit(ctx, pkgDHCP); err != nil {
		return err
	}

	return m.reload(ctx, dnsmasqInit)
}

// hostSection returns the dhcp host section for mac, creating one.
func (m *Manager) hostSection(ctx context.Context, mac string) (string, error) {
	if pkg, err := m.uci.Get(ctx, pkgDHCP); err == nil {
		for _, h := range pkg.OfType(typeHost) {
			for _, hm := range h.List("mac") {
				if strings.EqualFold(hm, mac) {
					return h.Name, nil
				}
			}
		}
	}

	return m.uci.AddAnonymous(ctx, pkgDHCP, typeHost)
}

// Block adds a firewall rule rejecting every packet from mac, in both the
// forward (to any zone) and input (to the router) direction.
func (m *Manager) Block(ctx context.Context, mac string) error {
	mac, ok := normMAC(mac)
	if !ok {
		return fmt.Errorf("%w: %q", errBadMAC, mac)
	}

	zone := m.lanZone(ctx)
	name := ruleName(mac)

	rules := map[string]map[string]any{
		name:         blockRule(zone, mac, "wrtgram block "+mac, true),
		name + "_in": blockRule(zone, mac, "wrtgram block "+mac+" (router)", false),
	}

	for section, values := range rules {
		if err := m.uci.AddSection(ctx, pkgFirewall, typeRule, section); err != nil {
			continue // the rule exists already; Set below refreshes it
		}

		if err := m.uci.Set(ctx, pkgFirewall, section, values); err != nil {
			return err
		}
	}

	if err := m.uci.Commit(ctx, pkgFirewall); err != nil {
		return err
	}

	return m.reload(ctx, firewallInit)
}

// Unblock removes the rules added by Block.
func (m *Manager) Unblock(ctx context.Context, mac string) error {
	mac, ok := normMAC(mac)
	if !ok {
		return fmt.Errorf("%w: %q", errBadMAC, mac)
	}

	name := ruleName(mac)

	for _, section := range []string{name, name + "_in"} {
		if err := m.uci.Delete(ctx, pkgFirewall, section); err != nil {
			continue // already gone
		}
	}

	if err := m.uci.Commit(ctx, pkgFirewall); err != nil {
		return err
	}

	return m.reload(ctx, firewallInit)
}

// lanZone finds the firewall zone that holds the "lan" network.
func (m *Manager) lanZone(ctx context.Context) string {
	pkg, err := m.uci.Get(ctx, pkgFirewall)
	if err != nil {
		return lanZone
	}

	for _, z := range pkg.OfType(typeZone) {
		for _, n := range z.List("network") {
			if n == lanZone {
				return z.Opt("name", lanZone)
			}
		}
	}

	return lanZone
}

func (m *Manager) reload(ctx context.Context, init string) error {
	if _, err := m.run.Run(ctx, init, "reload"); err != nil {
		return fmt.Errorf("%s reload: %w", init, err)
	}

	return nil
}

func ruleName(mac string) string { return rulePrefix + strings.ReplaceAll(mac, ":", "") }

// blockRule builds a fw4 rule rejecting mac; forward rules target every
// zone ("*"), the other variant guards the router itself (input).
func blockRule(zone, mac, title string, forward bool) map[string]any {
	v := map[string]any{"name": title, "src": zone, "src_mac": mac, "proto": protoAll, "target": targetReject, optEnabled: "1"}
	if forward {
		v["dest"] = "*"
	}

	return v
}

// normMAC validates a 48-bit MAC and returns it lower-case, colon separated.
func normMAC(s string) (string, bool) {
	const macBytes = 6

	hw, err := net.ParseMAC(strings.TrimSpace(s))
	if err != nil || len(hw) != macBytes {
		return s, false
	}

	return hw.String(), true
}

// hostName makes a dnsmasq-safe host name: letters, digits and dashes.
func hostName(name, mac string) string {
	var b strings.Builder

	for _, c := range strings.TrimSpace(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
			b.WriteRune(c)
		case c == ' ', c == '_', c == '.':
			b.WriteByte('-')
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "device-" + strings.ReplaceAll(mac[len(mac)-5:], ":", "")
	}

	return out
}
