// Package dhcp reads dnsmasq leases and rpcd host hints.
package dhcp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// DefaultLeaseFile is dnsmasq's default, relative to the filesystem root.
const DefaultLeaseFile = "tmp/dhcp.leases"

const leaseFields = 4

var errLeaseFile = errors.New("lease file")

// Leases implements usecase.Leases from the dnsmasq lease file.
type Leases struct {
	fsys fs.FS
	path string // relative to fsys
}

// NewLeases creates the adapter; path "" means the default.
func NewLeases(fsys fs.FS, path string) *Leases {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = DefaultLeaseFile
	}

	return &Leases{fsys: fsys, path: path}
}

// Exists reports whether the lease file is present.
func (l *Leases) Exists() bool {
	_, err := fs.Stat(l.fsys, l.path)

	return err == nil
}

// Leases implements usecase.Leases.
func (l *Leases) Leases(context.Context) ([]entity.Lease, error) {
	raw, err := fs.ReadFile(l.fsys, l.path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errLeaseFile, err)
	}

	return Parse(string(raw)), nil
}

// Parse decodes lease lines: "<expiry> <mac> <ip> <hostname> <client-id>".
func Parse(text string) []entity.Lease {
	var out []entity.Lease

	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < leaseFields {
			continue
		}

		lease := entity.Lease{MAC: strings.ToLower(f[1]), IP: f[2]}

		if exp, err := strconv.ParseInt(f[0], 10, 64); err == nil && exp > 0 {
			lease.Expires = time.Unix(exp, 0)
		}

		if f[3] != "*" {
			lease.Hostname = f[3]
		}

		out = append(out, lease)
	}

	return out
}

// Hints implements usecase.HostHints with rpcd's luci-rpc getHostHints,
// which merges ARP, DHCP and static host entries. Absent rpcd-mod-luci is
// not an error: the map is just empty.
type Hints struct {
	ubus usecase.Ubus
}

// NewHints creates the adapter.
func NewHints(ubus usecase.Ubus) *Hints { return &Hints{ubus: ubus} }

// Hints implements usecase.HostHints: MAC → name.
func (h *Hints) Hints(ctx context.Context) (map[string]string, error) {
	var reply map[string]struct {
		Name string `json:"name"`
	}

	if err := h.ubus.Call(ctx, "luci-rpc", "getHostHints", nil, &reply); err != nil {
		return map[string]string{}, nil //nolint:nilerr // optional source
	}

	out := make(map[string]string, len(reply))

	for mac, v := range reply {
		if v.Name != "" {
			out[strings.ToLower(mac)] = strings.TrimSuffix(v.Name, ".lan")
		}
	}

	return out, nil
}
