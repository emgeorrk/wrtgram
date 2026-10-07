package entity

import "time"

// WANStatus is the state of the upstream interface (ubus `network.interface.<wan> status`).
type WANStatus struct {
	Interface string // netifd logical name, e.g. "wan"
	Device    string // l3 device, e.g. "eth0"
	Gateway   string
	Proto     string
	IPv4      []string
	DNS       []string
	Uptime    time.Duration
	Up        bool
}

// Device is a LAN client assembled from DHCP leases and Wi-Fi associations.
type Device struct {
	Expires  time.Time
	MAC      string // lower-case, colon separated
	IP       string
	Hostname string
	Iface    string // Wi-Fi interface the client is associated to, "" when wired or offline
	Signal   int    // dBm, 0 when unknown
	Wireless bool
}
