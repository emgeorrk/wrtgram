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
	Band     string // "2.4 GHz", "5 GHz", "6 GHz" or ""
	Signal   int    // dBm, 0 when unknown
	Wireless bool
	Static   bool // has a static DHCP lease
	Blocked  bool // has a wrtgram firewall block rule
}

// Lease is one DHCP lease.
type Lease struct {
	Expires  time.Time
	MAC      string
	IP       string
	Hostname string // "" when the client sent none
}

// WifiClient is a station associated to an access point interface.
type WifiClient struct {
	MAC    string
	Iface  string // hostapd interface, e.g. "phy1-ap0"
	Freq   int    // MHz, 0 when unknown
	Signal int    // dBm, 0 when unknown
}

// Frequency boundaries between the Wi-Fi bands, MHz.
const (
	band5GHzFrom = 3000
	band6GHzFrom = 5900
)

// Band renders the frequency as "2.4 GHz", "5 GHz" or "6 GHz".
func (c WifiClient) Band() string {
	switch {
	case c.Freq == 0:
		return ""
	case c.Freq < band5GHzFrom:
		return "2.4 GHz"
	case c.Freq < band6GHzFrom:
		return "5 GHz"
	}

	return "6 GHz"
}
