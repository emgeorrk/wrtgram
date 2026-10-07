package devices_test

import (
	"testing"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase/devices"
)

func TestMerge(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	leases := []entity.Lease{
		{MAC: "AA:BB:CC:DD:EE:03", IP: "192.168.1.222", Hostname: "Mac", Expires: now.Add(time.Hour)},
		{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.144", Expires: now.Add(time.Hour)},
		{MAC: "aa:bb:cc:dd:ee:05", IP: "192.168.1.5", Hostname: "old", Expires: now.Add(-time.Hour)},
	}
	clients := []entity.WifiClient{
		{MAC: "aa:bb:cc:dd:ee:01", Iface: "phy1-ap0", Freq: 5200, Signal: -60},
		{MAC: "aa:bb:cc:dd:ee:09", Iface: "phy0-ap0", Freq: 2437},
	}
	hints := map[string]string{"aa:bb:cc:dd:ee:01": "wlan0", "aa:bb:cc:dd:ee:09": "tv", "aa:bb:cc:dd:ee:03": "ignored"}

	got := devices.Merge(leases, clients, hints, now)

	want := []struct {
		mac, ip, name, band string
		wireless            bool
	}{
		{"aa:bb:cc:dd:ee:01", "192.168.1.144", "wlan0", "5 GHz", true},
		{"aa:bb:cc:dd:ee:03", "192.168.1.222", "Mac", "", false},
		{"aa:bb:cc:dd:ee:09", "", "tv", "2.4 GHz", true},
	}

	if len(got) != len(want) {
		t.Fatalf("Merge() = %d devices, want %d: %+v", len(got), len(want), got)
	}

	for i, w := range want {
		d := got[i]
		if d.MAC != w.mac || d.IP != w.ip || d.Hostname != w.name || d.Band != w.band || d.Wireless != w.wireless {
			t.Errorf("device %d = %+v, want %+v", i, d, w)
		}
	}
}
