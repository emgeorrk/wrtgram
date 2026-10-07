package entity

import "time"

// TunnelProto is the netifd protocol of a VPN interface.
type TunnelProto string

// Supported tunnel protocols.
const (
	TunnelWireGuard TunnelProto = "wireguard"
	TunnelAmneziaWG TunnelProto = "amneziawg"
)

// TunnelRef identifies a configured tunnel interface.
type TunnelRef struct {
	Name   string // netifd logical interface, e.g. "awg0"
	Device string // kernel device, usually the same name
	Proto  TunnelProto
}

// Peer is one WireGuard-style peer of a tunnel.
type Peer struct {
	LastHandshake time.Time // zero when never
	PublicKey     string
	Endpoint      string
	AllowedIPs    []string
	RxBytes       uint64
	TxBytes       uint64
}

// Tunnel is the live state of a tunnel interface.
type Tunnel struct {
	Ref    TunnelRef
	Peers  []Peer
	Uptime time.Duration
	Up     bool
}

// Handshaked reports whether any peer completed a handshake within the window.
func (t Tunnel) Handshaked(now time.Time, within time.Duration) bool {
	for _, p := range t.Peers {
		if !p.LastHandshake.IsZero() && now.Sub(p.LastHandshake) <= within {
			return true
		}
	}

	return false
}
