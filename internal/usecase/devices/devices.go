// Package devices assembles the list of LAN clients from DHCP leases,
// Wi-Fi associations and rpcd host hints.
package devices

import (
	"context"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// Service is the devices usecase. Any source may be nil.
type Service struct {
	leases usecase.Leases
	wifi   usecase.Wireless
	hints  usecase.HostHints
	clock  usecase.Clock
}

// New creates the service.
func New(leases usecase.Leases, wifi usecase.Wireless, hints usecase.HostHints, clock usecase.Clock) *Service {
	return &Service{leases: leases, wifi: wifi, hints: hints, clock: clock}
}

// List returns the merged, sorted device list.
func (s *Service) List(ctx context.Context) ([]entity.Device, error) {
	var (
		leases  []entity.Lease
		clients []entity.WifiClient
		hints   map[string]string
		err     error
	)

	if s.leases != nil {
		if leases, err = s.leases.Leases(ctx); err != nil {
			return nil, err
		}
	}

	if s.wifi != nil {
		if clients, err = s.wifi.Clients(ctx); err != nil {
			return nil, err
		}
	}

	if s.hints != nil {
		if h, err := s.hints.Hints(ctx); err == nil {
			hints = h
		}
	}

	return Merge(leases, clients, hints, s.clock.Now()), nil
}

// Merge joins the sources by MAC. Expired leases are kept only when the
// client is still associated to Wi-Fi.
func Merge(leases []entity.Lease, clients []entity.WifiClient, hints map[string]string, now time.Time) []entity.Device {
	byMAC := make(map[string]*entity.Device, len(leases)+len(clients))

	for _, l := range leases {
		mac := strings.ToLower(l.MAC)
		byMAC[mac] = &entity.Device{Expires: l.Expires, MAC: mac, IP: l.IP, Hostname: l.Hostname}
	}

	for _, c := range clients {
		mac := strings.ToLower(c.MAC)

		d, ok := byMAC[mac]
		if !ok {
			d = &entity.Device{MAC: mac}
			byMAC[mac] = d
		}

		d.Wireless, d.Iface, d.Band, d.Signal = true, c.Iface, c.Band(), c.Signal
	}

	out := make([]entity.Device, 0, len(byMAC))

	for _, d := range byMAC {
		if d.Hostname == "" {
			d.Hostname = hints[d.MAC]
		}

		if !d.Wireless && !d.Expires.IsZero() && d.Expires.Before(now) {
			continue
		}

		out = append(out, *d)
	}

	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })

	return out
}

// less orders by IPv4 address, devices without an address last, then by MAC.
func less(a, b entity.Device) bool {
	ia, ib := net.ParseIP(a.IP).To4(), net.ParseIP(b.IP).To4()

	switch {
	case ia != nil && ib != nil:
		if c := strings.Compare(string(ia), string(ib)); c != 0 {
			return c < 0
		}
	case ia != nil:
		return true
	case ib != nil:
		return false
	}

	return a.MAC < b.MAC
}
