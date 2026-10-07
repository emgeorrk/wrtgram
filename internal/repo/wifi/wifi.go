// Package wifi lists associated stations through hostapd's ubus objects.
// Object names differ between releases (hostapd.wlan0, hostapd.phy0-ap0),
// so they are always enumerated with `ubus list`.
package wifi

import (
	"context"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

const prefix = "hostapd."

// Hostapd implements usecase.Wireless.
type Hostapd struct {
	ubus usecase.Ubus
}

// New creates the adapter.
func New(ubus usecase.Ubus) *Hostapd { return &Hostapd{ubus: ubus} }

// Interfaces returns the access point interface names.
func (h *Hostapd) Interfaces(ctx context.Context) ([]string, error) {
	objs, err := h.ubus.List(ctx, prefix+"*")
	if err != nil {
		return nil, err
	}

	var out []string

	for _, o := range objs {
		if name, ok := strings.CutPrefix(o, prefix); ok && name != "" {
			out = append(out, name)
		}
	}

	return out, nil
}

type clientsReply struct {
	Clients map[string]struct {
		Signal     int  `json:"signal"`
		Authorized bool `json:"authorized"`
		Assoc      bool `json:"assoc"`
	} `json:"clients"`
	Freq int `json:"freq"`
}

// Clients implements usecase.Wireless.
func (h *Hostapd) Clients(ctx context.Context) ([]entity.WifiClient, error) {
	ifaces, err := h.Interfaces(ctx)
	if err != nil {
		return nil, err
	}

	var out []entity.WifiClient

	for _, iface := range ifaces {
		var reply clientsReply
		if err := h.ubus.Call(ctx, prefix+iface, "get_clients", nil, &reply); err != nil {
			continue // an AP going down mid-scan is not an error
		}

		for mac, c := range reply.Clients {
			if !c.Assoc && !c.Authorized {
				continue
			}

			out = append(out, entity.WifiClient{MAC: strings.ToLower(mac), Iface: iface, Freq: reply.Freq, Signal: c.Signal})
		}
	}

	return out, nil
}
