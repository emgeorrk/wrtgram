package entity

import "time"

// Board describes the router hardware and firmware (ubus `system board`).
type Board struct {
	Model    string
	Hostname string
	Release  string // e.g. "OpenWrt 25.12.5 r33051-f5dae5ece4"
	Kernel   string
	Target   string // e.g. "mediatek/filogic"
	Arch     string // e.g. "aarch64_cortex-a53"
}

// SystemInfo is a snapshot of the running system (ubus `system info` + statfs).
type SystemInfo struct {
	Uptime     time.Duration
	Load       [3]float64
	MemTotal   uint64
	MemAvail   uint64
	FlashTotal uint64
	FlashFree  uint64
}

// Temperature is one hwmon reading in degrees Celsius.
type Temperature struct {
	Sensor  string // hwmon name, e.g. "cpu_thermal" or "mt7981-thermal"
	Label   string // temp*_label when present
	Celsius float64
}

// Service is a procd service as reported by ubus `service list`.
type Service struct {
	Name      string
	Instances int
	Running   int
}
