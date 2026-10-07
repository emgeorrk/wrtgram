package entity

import "time"

// FailoverMode says where the default route currently points.
type FailoverMode int

// Failover modes.
const (
	FailoverPrimary FailoverMode = iota // routed via the first configured tunnel
	FailoverBackup                      // routed via a lower-priority tunnel
	FailoverDirect                      // no tunnel route, traffic leaves via WAN
)

// String implements fmt.Stringer.
func (m FailoverMode) String() string {
	switch m {
	case FailoverPrimary:
		return "primary"
	case FailoverBackup:
		return "backup"
	case FailoverDirect:
		return "direct"
	}

	return "unknown"
}

// TunnelHealth is the last probe result for one failover tunnel.
type TunnelHealth struct {
	DownSince time.Time // zero while healthy
	Name      string
	Healthy   bool
}

// FailoverState is the persisted state of the failover controller.
type FailoverState struct {
	Since      time.Time // when Mode/Dev last changed
	CheckedAt  time.Time
	Dev        string // tunnel owning the default route, "" when direct
	ServiceDev string // tunnel carrying the service networks, "" when none
	Tunnels    []TunnelHealth
	Mode       FailoverMode
	ManualOff  bool
}

// FailoverEventKind classifies a routing change.
type FailoverEventKind int

// Failover events.
const (
	FailoverPrimaryBack FailoverEventKind = iota // primary tunnel healthy again
	FailoverSwitched                             // moved to a backup tunnel
	FailoverAllDown                              // every tunnel down, routing direct
	FailoverTunnelUp                             // a tunnel came back while direct
)

// FailoverEvent describes one routing change for notifications.
type FailoverEvent struct {
	From     string // previous device, "" when direct
	To       string // new device, "" when direct
	Downtime time.Duration
	Kind     FailoverEventKind
}
