package entity

import "time"

// LoginKind is the login surface an event was observed on.
type LoginKind int

// Login surfaces.
const (
	LoginSSH LoginKind = iota
	LoginLuCI
)

// LoginEvent is a parsed authentication line from the system log.
type LoginEvent struct {
	At      time.Time
	IP      string
	User    string
	Method  string // "password", "pubkey" or ""
	Kind    LoginKind
	Success bool
}

// DHCPEvent is a dnsmasq hotplug event (ACTION, MACADDR, IPADDR, HOSTNAME).
type DHCPEvent struct {
	Action   string // "add", "update" or "remove"
	MAC      string
	IP       string
	Hostname string
}

// LogLine is one syslog entry as printed by `logread`.
type LogLine struct {
	At       time.Time
	Facility string // "authpriv.notice"
	Tag      string // "dropbear", "luci", … (without the pid)
	Message  string
}
