package entity

import (
	"errors"
	"fmt"
	"time"
)

var errBadMode = errors.New("unknown failover mode")

// FailoverMode says where the default route currently points.
type FailoverMode int

// Failover modes.
const (
	FailoverPrimary FailoverMode = iota // routed via the first configured tunnel
	FailoverBackup                      // routed via a lower-priority tunnel
	FailoverDirect                      // no tunnel route, traffic leaves via WAN
)

// MarshalText renders the mode by name in JSON state files.
func (m FailoverMode) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

// UnmarshalText parses the name written by MarshalText.
func (m *FailoverMode) UnmarshalText(b []byte) error {
	for _, c := range []FailoverMode{FailoverPrimary, FailoverBackup, FailoverDirect} {
		if c.String() == string(b) {
			*m = c

			return nil
		}
	}

	return fmt.Errorf("%w: %q", errBadMode, b)
}

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
	DownSince time.Time `json:"down_since"` // zero while healthy
	Name      string    `json:"name"`
	Healthy   bool      `json:"healthy"`
}

// FailoverState is the persisted state of the failover controller.
type FailoverState struct {
	Since      time.Time      `json:"since"` // when Mode/Dev last changed
	CheckedAt  time.Time      `json:"checked_at"`
	Dev        string         `json:"dev"`         // tunnel owning the default route, "" when direct
	ServiceDev string         `json:"service_dev"` // tunnel carrying the service networks, "" when none
	Tunnels    []TunnelHealth `json:"tunnels"`
	Mode       FailoverMode   `json:"mode"`
	ManualOff  bool           `json:"manual_off"`
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
