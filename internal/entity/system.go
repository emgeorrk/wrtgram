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
	Detail    string // optional status from the service's own reporting (e.g. adblock)
	Instances int
	Running   int
	Failed    int // instances that should run (respawn) or exited with an error
}

// State classifies the service for display.
func (s Service) State() ServiceState {
	switch {
	case s.Failed > 0:
		return ServiceFailed
	case s.Instances == 0:
		return ServiceIdle
	case s.Running == s.Instances:
		return ServiceRunning
	}

	return ServiceDone // one-shot instances that finished successfully
}

// ServiceState is the display classification of a procd service.
type ServiceState int

// Service states.
const (
	ServiceRunning ServiceState = iota
	ServiceDone
	ServiceIdle
	ServiceFailed
)
