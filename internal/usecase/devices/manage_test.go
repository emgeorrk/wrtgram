package devices_test

import (
	"context"
	"strings"
	"testing"

	"github.com/emgeorrk/wrtgram/internal/repo/execx"
	"github.com/emgeorrk/wrtgram/internal/repo/ubus"
	"github.com/emgeorrk/wrtgram/internal/repo/uci"
	"github.com/emgeorrk/wrtgram/internal/usecase/devices"
)

const (
	dhcpPkg = `{"values": {"cfg0": {".type": "dnsmasq", ".name": "cfg0", ".index": 0},
		"known": {".type": "host", ".name": "known", ".index": 1, "name": "nas", "mac": "aa:bb:cc:dd:ee:03", "ip": "192.168.1.222"}}}`
	fwPkg = `{"values": {"lanz": {".type": "zone", ".name": "lanz", ".index": 0, "name": "lan", "network": ["lan", "guest"]},
		"wrtgram_block_aabbccddee09": {".type": "rule", ".name": "wrtgram_block_aabbccddee09", ".index": 1, "src_mac": "aa:bb:cc:dd:ee:09", "enabled": "1"}}}`
)

func newManager() (*devices.Manager, *execx.Fake) {
	fake := execx.NewFake().
		OnString(`ubus call uci get {"config":"dhcp"}`, dhcpPkg).
		OnString(`ubus call uci get {"config":"firewall"}`, fwPkg).
		OnString(`ubus call uci add {"config":"dhcp","type":"host"}`, `{"section": "cfg9"}`).
		OnString("ubus call uci add", "").
		OnString("ubus call uci set", "").
		OnString("ubus call uci delete", "").
		OnString("ubus call uci commit", "").
		OnString("/etc/init.d/dnsmasq reload", "").
		OnString("/etc/init.d/firewall reload", "")

	return devices.NewManager(uci.New(ubus.New(fake)), fake), fake
}

func TestFlags(t *testing.T) {
	t.Parallel()

	m, _ := newManager()

	static, blocked := m.Flags(context.Background())
	if !static["aa:bb:cc:dd:ee:03"] || !blocked["aa:bb:cc:dd:ee:09"] || len(static) != 1 || len(blocked) != 1 {
		t.Errorf("Flags() = %v %v", static, blocked)
	}
}

func TestRemember(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mac, ip  string
		host     string
		wantSect string
		wantErr  bool
	}{
		{name: "new host", mac: "AA:BB:CC:DD:EE:05", ip: "192.168.1.50", host: "My iPhone", wantSect: "cfg9"},
		{name: "existing host updated", mac: "aa:bb:cc:dd:ee:03", ip: "192.168.1.223", host: "", wantSect: "known"},
		{name: "bad mac", mac: "nope", ip: "1.2.3.4", wantErr: true},
		{name: "bad ip", mac: "aa:bb:cc:dd:ee:05", ip: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, fake := newManager()

			err := m.Remember(context.Background(), tt.mac, tt.ip, tt.host)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Remember() err = %v", err)
			}

			if tt.wantErr {
				return
			}

			calls := strings.Join(fake.Calls(), "\n")
			if !strings.Contains(calls, `"section":"`+tt.wantSect+`"`) || !strings.Contains(calls, `"ip":"`+tt.ip+`"`) ||
				!strings.Contains(calls, `"mac":"`+strings.ToLower(tt.mac)+`"`) || !strings.Contains(calls, "/etc/init.d/dnsmasq reload") {
				t.Errorf("unexpected calls:\n%s", calls)
			}

			if tt.host != "" && !strings.Contains(calls, `"name":"My-iPhone"`) {
				t.Errorf("host name not sanitised:\n%s", calls)
			}
		})
	}
}

func TestBlockUnblock(t *testing.T) {
	t.Parallel()

	m, fake := newManager()
	ctx := context.Background()

	if err := m.Block(ctx, "AA:BB:CC:DD:EE:07"); err != nil {
		t.Fatalf("Block: %v", err)
	}

	calls := strings.Join(fake.Calls(), "\n")
	for _, want := range []string{
		`"name":"wrtgram_block_aabbccddee07","type":"rule"`,
		`"dest":"*"`, `"src":"lan"`, `"src_mac":"aa:bb:cc:dd:ee:07"`, `"target":"REJECT"`,
		`"section":"wrtgram_block_aabbccddee07_in"`,
		`ubus call uci commit {"config":"firewall"}`, "/etc/init.d/firewall reload",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("Block: missing %q in:\n%s", want, calls)
		}
	}

	if err := m.Unblock(ctx, "aa:bb:cc:dd:ee:07"); err != nil {
		t.Fatalf("Unblock: %v", err)
	}

	calls = strings.Join(fake.Calls(), "\n")
	if !strings.Contains(calls, `ubus call uci delete {"config":"firewall","section":"wrtgram_block_aabbccddee07"}`) {
		t.Errorf("Unblock: rule not deleted:\n%s", calls)
	}
}
