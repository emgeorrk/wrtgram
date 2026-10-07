package netifd_test

import (
	"context"
	"testing"

	"github.com/emgeorrk/wrtgram/internal/repo/execx"
	"github.com/emgeorrk/wrtgram/internal/repo/netifd"
	"github.com/emgeorrk/wrtgram/internal/repo/ubus"
)

const dump = `{"interface": [
	{"interface": "lan", "up": true, "l3_device": "br-lan", "proto": "static", "device": "br-lan", "uptime": 10, "ipv4-address": [{"address": "192.168.1.1", "mask": 24}], "route": [], "dns-server": []},
	{"interface": "wwan", "up": true, "l3_device": "wwan0", "proto": "dhcp", "device": "wwan0", "uptime": 20, "ipv4-address": [{"address": "10.9.9.2", "mask": 24}], "route": [{"target": "0.0.0.0", "mask": 0, "nexthop": "10.9.9.1"}], "dns-server": ["1.1.1.1"]},
	{"interface": "wan", "up": true, "l3_device": "eth0", "proto": "dhcp", "device": "eth0", "uptime": 30, "ipv4-address": [{"address": "10.0.0.2", "mask": 24}], "route": [{"target": "0.0.0.0", "mask": 0, "nexthop": "10.0.0.1"}], "dns-server": ["8.8.8.8"]}
]}`

func TestStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		iface    string
		dump     string
		wantName string
		wantGW   string
		wantErr  bool
	}{
		{name: "auto prefers wan", dump: dump, wantName: "wan", wantGW: "10.0.0.1"},
		{name: "auto falls back to any default route", dump: `{"interface": [` +
			`{"interface": "wwan", "up": true, "l3_device": "wwan0", "proto": "dhcp", "route": [{"target": "0.0.0.0", "nexthop": "10.9.9.1"}]}]}`,
			wantName: "wwan", wantGW: "10.9.9.1"},
		{name: "wan down but named wan", dump: `{"interface": [{"interface": "wan", "up": false, "proto": "dhcp", "device": "eth0"}]}`, wantName: "wan"},
		{name: "nothing upstream", dump: `{"interface": [{"interface": "lan", "up": true, "proto": "static"}]}`, wantErr: true},
		{name: "explicit interface", iface: "wwan", dump: dump, wantName: "wwan", wantGW: "10.9.9.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := execx.NewFake().OnString("ubus call network.interface dump", tt.dump).
				OnString("ubus call network.interface.wwan status", `{"up": true, "l3_device": "wwan0", "proto": "dhcp", "uptime": 20, "ipv4-address": [{"address": "10.9.9.2"}], "route": [{"target": "0.0.0.0", "nexthop": "10.9.9.1"}]}`)

			st, err := netifd.NewWAN(ubus.New(fake), tt.iface).Status(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Status() err = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if st.Interface != tt.wantName || st.Gateway != tt.wantGW {
				t.Errorf("Status() = %+v", st)
			}
		})
	}
}
