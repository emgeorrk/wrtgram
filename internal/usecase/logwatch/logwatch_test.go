package logwatch_test

import (
	"testing"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase/logwatch"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tag     string
		msg     string
		wantOK  bool
		wantEv  entity.LoginEvent
	}{
		{name: "pubkey", tag: "dropbear", msg: "Pubkey auth succeeded for 'root' with ssh-ed25519 key SHA256:abc from 192.168.1.222:50661",
			wantOK: true, wantEv: entity.LoginEvent{IP: "192.168.1.222", User: "root", Method: "key", Kind: entity.LoginSSH, Success: true}},
		{name: "password", tag: "dropbear", msg: "Password auth succeeded for 'root' from 10.0.0.5:1234",
			wantOK: true, wantEv: entity.LoginEvent{IP: "10.0.0.5", User: "root", Method: "password", Kind: entity.LoginSSH, Success: true}},
		{name: "bad password", tag: "dropbear", msg: "Bad password attempt for 'root' from 203.0.113.9:4444",
			wantOK: true, wantEv: entity.LoginEvent{IP: "203.0.113.9", User: "root", Kind: entity.LoginSSH}},
		{name: "nonexistent user", tag: "dropbear", msg: "Login attempt for nonexistent user from 203.0.113.9:4445",
			wantOK: true, wantEv: entity.LoginEvent{IP: "203.0.113.9", Kind: entity.LoginSSH}},
		{name: "ipv6 bare", tag: "dropbear", msg: "Bad password attempt for 'admin' from fe80::1:2222",
			wantOK: true, wantEv: entity.LoginEvent{IP: "fe80::1", User: "admin", Kind: entity.LoginSSH}},
		{name: "ipv6 bracketed", tag: "dropbear", msg: "Password auth succeeded for 'root' from [2001:db8::7]:22",
			wantOK: true, wantEv: entity.LoginEvent{IP: "2001:db8::7", User: "root", Method: "password", Kind: entity.LoginSSH, Success: true}},
		{name: "dropbear noise", tag: "dropbear", msg: "Child connection from 192.168.1.222:50661"},
		{name: "luci ok", tag: "luci", msg: "luci: accepted login on /admin for root from 192.168.1.213",
			wantOK: true, wantEv: entity.LoginEvent{IP: "192.168.1.213", User: "root", Kind: entity.LoginLuCI, Success: true}},
		{name: "luci fail ipv6", tag: "luci", msg: "luci: failed login on /admin for root from 2001:db8::1",
			wantOK: true, wantEv: entity.LoginEvent{IP: "2001:db8::1", User: "root", Kind: entity.LoginLuCI}},
		{name: "other", tag: "dnsmasq-dhcp", msg: "DHCPACK(br-lan) 192.168.1.5 aa:bb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ev, ok := logwatch.Parse(entity.LogLine{Tag: tt.tag, Message: tt.msg})
			if ok != tt.wantOK {
				t.Fatalf("Parse() ok = %v, want %v", ok, tt.wantOK)
			}

			if !ok {
				return
			}

			if ev.IP != tt.wantEv.IP || ev.User != tt.wantEv.User || ev.Method != tt.wantEv.Method ||
				ev.Kind != tt.wantEv.Kind || ev.Success != tt.wantEv.Success {
				t.Errorf("Parse() = %+v, want %+v", ev, tt.wantEv)
			}
		})
	}
}
