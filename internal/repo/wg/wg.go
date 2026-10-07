// Package wg reads WireGuard and AmneziaWG tunnels: the list from netifd,
// the live peers from `wg show <dev> dump` / `awg show <dev> dump`.
package wg

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

var errNoTool = errors.New("neither wg nor awg found")

const (
	toolWG  = "wg"
	toolAWG = "awg"
)

const (
	peerFields   = 8
	fieldPub     = 0
	fieldEnd     = 2
	fieldAllowed = 3
	fieldHS      = 4
	fieldRx      = 5
	fieldTx      = 6
)

// Tunnels implements usecase.Tunnels.
type Tunnels struct {
	run  usecase.Runner
	ubus usecase.Ubus
}

// New creates the adapter.
func New(run usecase.Runner, ubus usecase.Ubus) *Tunnels { return &Tunnels{run: run, ubus: ubus} }

// Available reports whether a wg/awg tool exists.
func (t *Tunnels) Available() bool { return t.run.LookPath(toolWG) || t.run.LookPath(toolAWG) }

type dumpReply struct {
	Interface []struct {
		Interface string `json:"interface"`
		L3Device  string `json:"l3_device"`
		Proto     string `json:"proto"`
	} `json:"interface"`
}

// List implements usecase.Tunnels: every netifd interface with a WireGuard
// family protocol, in netifd order.
func (t *Tunnels) List(ctx context.Context) ([]entity.TunnelRef, error) {
	var reply dumpReply
	if err := t.ubus.Call(ctx, "network.interface", "dump", nil, &reply); err != nil {
		return nil, err
	}

	var out []entity.TunnelRef

	for _, i := range reply.Interface {
		proto := entity.TunnelProto(i.Proto)
		if proto != entity.TunnelWireGuard && proto != entity.TunnelAmneziaWG {
			continue
		}

		dev := i.L3Device
		if dev == "" {
			dev = i.Interface
		}

		out = append(out, entity.TunnelRef{Name: i.Interface, Device: dev, Proto: proto})
	}

	return out, nil
}

type ifaceStatus struct {
	Uptime int64 `json:"uptime"`
	Up     bool  `json:"up"`
}

// Status implements usecase.Tunnels.
func (t *Tunnels) Status(ctx context.Context, ref entity.TunnelRef) (entity.Tunnel, error) {
	tun := entity.Tunnel{Ref: ref}

	var st ifaceStatus
	if err := t.ubus.Call(ctx, "network.interface."+ref.Name, "status", nil, &st); err == nil {
		tun.Up = st.Up
		tun.Uptime = time.Duration(st.Uptime) * time.Second
	}

	tool, err := t.tool(ref.Proto)
	if err != nil {
		return tun, err
	}

	raw, err := t.run.Run(ctx, tool, "show", ref.Device, "dump")
	if err != nil {
		// The device is absent while the interface is down: no peers, not an error.
		return tun, nil //nolint:nilerr // a missing device simply means "down"
	}

	tun.Peers = ParsePeers(string(raw))

	return tun, nil
}

func (t *Tunnels) tool(proto entity.TunnelProto) (string, error) {
	prefer, other := toolWG, toolAWG
	if proto == entity.TunnelAmneziaWG {
		prefer, other = toolAWG, toolWG
	}

	switch {
	case t.run.LookPath(prefer):
		return prefer, nil
	case t.run.LookPath(other):
		return other, nil
	}

	return "", errNoTool
}

// ParsePeers decodes `show <dev> dump`: the first line describes the
// interface (private key first — never logged, never parsed), every further
// line is a peer with 8 tab-separated fields: public-key, preshared-key,
// endpoint, allowed-ips, latest-handshake, transfer-rx, transfer-tx,
// persistent-keepalive. AmneziaWG appends extra fields to the interface line
// only, so the peer format is shared.
func ParsePeers(dump string) []entity.Peer {
	var peers []entity.Peer

	for i, line := range strings.Split(strings.TrimSpace(dump), "\n") {
		if i == 0 {
			continue
		}

		f := strings.Split(line, "\t")
		if len(f) < peerFields {
			continue
		}

		p := entity.Peer{PublicKey: f[fieldPub], Endpoint: f[fieldEnd]}

		if f[fieldAllowed] != "(none)" {
			p.AllowedIPs = strings.Split(f[fieldAllowed], ",")
		}

		if hs, err := strconv.ParseInt(f[fieldHS], 10, 64); err == nil && hs > 0 {
			p.LastHandshake = time.Unix(hs, 0)
		}

		if v, err := strconv.ParseUint(f[fieldRx], 10, 64); err == nil {
			p.RxBytes = v
		}

		if v, err := strconv.ParseUint(f[fieldTx], 10, 64); err == nil {
			p.TxBytes = v
		}

		peers = append(peers, p)
	}

	return peers
}

// String renders a tunnel reference for logs.
func String(ref entity.TunnelRef) string { return fmt.Sprintf("%s (%s)", ref.Name, ref.Proto) }
