// Package netifd reads interface state from netifd's ubus objects.
package netifd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

var errNoWAN = errors.New("no upstream interface found")

const (
	wanName      = "wan"
	defaultRoute = "0.0.0.0"
)

// WAN implements usecase.WAN. With an empty name the upstream interface is
// detected as the first one that is up and owns a default route, preferring
// the one called "wan".
type WAN struct {
	ubus usecase.Ubus
	name string
}

// NewWAN creates the adapter; name may be "" for auto-detection.
func NewWAN(ubus usecase.Ubus, name string) *WAN { return &WAN{ubus: ubus, name: name} }

type ifaceStatus struct {
	Interface string `json:"interface"`
	L3Device  string `json:"l3_device"`
	Device    string `json:"device"`
	Proto     string `json:"proto"`
	IPv4      []struct {
		Address string `json:"address"`
	} `json:"ipv4-address"`
	Route []struct {
		Target  string `json:"target"`
		Nexthop string `json:"nexthop"`
	} `json:"route"`
	DNS    []string `json:"dns-server"`
	Uptime int64    `json:"uptime"`
	Up     bool     `json:"up"`
}

// Status implements usecase.WAN.
func (w *WAN) Status(ctx context.Context) (entity.WANStatus, error) {
	if w.name != "" {
		var st ifaceStatus
		if err := w.ubus.Call(ctx, "network.interface."+w.name, "status", nil, &st); err != nil {
			return entity.WANStatus{}, err
		}

		st.Interface = w.name

		return toEntity(st), nil
	}

	var dump struct {
		Interface []ifaceStatus `json:"interface"`
	}

	if err := w.ubus.Call(ctx, "network.interface", "dump", nil, &dump); err != nil {
		return entity.WANStatus{}, err
	}

	if st, ok := pickWAN(dump.Interface); ok {
		return toEntity(st), nil
	}

	return entity.WANStatus{}, errNoWAN
}

func pickWAN(ifaces []ifaceStatus) (ifaceStatus, bool) {
	var candidate *ifaceStatus

	for i := range ifaces {
		st := &ifaces[i]
		if !hasDefaultRoute(*st) {
			continue
		}

		if st.Interface == wanName {
			return *st, true
		}

		if candidate == nil {
			candidate = st
		}
	}

	// Nothing has a default route (link down): fall back to the one named wan.
	if candidate == nil {
		for i := range ifaces {
			if ifaces[i].Interface == wanName {
				return ifaces[i], true
			}
		}

		return ifaceStatus{}, false
	}

	return *candidate, true
}

func hasDefaultRoute(st ifaceStatus) bool {
	for _, r := range st.Route {
		if r.Target == defaultRoute || r.Target == "::" {
			return true
		}
	}

	return false
}

func toEntity(st ifaceStatus) entity.WANStatus {
	out := entity.WANStatus{
		Interface: st.Interface,
		Device:    st.L3Device,
		Proto:     st.Proto,
		DNS:       st.DNS,
		Uptime:    time.Duration(st.Uptime) * time.Second,
		Up:        st.Up,
	}

	if out.Device == "" {
		out.Device = st.Device
	}

	for _, a := range st.IPv4 {
		out.IPv4 = append(out.IPv4, a.Address)
	}

	for _, r := range st.Route {
		if r.Target == defaultRoute {
			out.Gateway = r.Nexthop

			break
		}
	}

	return out
}

// Gateway is a convenience for the failover: the IPv4 next hop and device.
func (w *WAN) Gateway(ctx context.Context) (gw, dev string, err error) {
	st, err := w.Status(ctx)
	if err != nil {
		return "", "", fmt.Errorf("wan gateway: %w", err)
	}

	return st.Gateway, st.Device, nil
}
