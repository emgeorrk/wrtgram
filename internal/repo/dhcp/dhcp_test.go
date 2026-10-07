package dhcp_test

import (
	"testing"

	"github.com/emgeorrk/wrtgram/internal/repo/dhcp"
)

func TestParse(t *testing.T) {
	t.Parallel()

	leases := dhcp.Parse("1791405565 AA:BB:CC:DD:EE:01 192.168.1.144 wlan0 *\n" +
		"1791401563 aa:bb:cc:dd:ee:02 192.168.1.236 * 01:aa:bb\n" +
		"garbage\n")

	if len(leases) != 2 {
		t.Fatalf("Parse() = %d leases, want 2", len(leases))
	}

	if leases[0].MAC != "aa:bb:cc:dd:ee:01" || leases[0].Hostname != "wlan0" || leases[0].Expires.Unix() != 1791405565 {
		t.Errorf("lease 0 = %+v", leases[0])
	}

	if leases[1].Hostname != "" {
		t.Errorf("'*' hostname must be empty: %+v", leases[1])
	}
}
