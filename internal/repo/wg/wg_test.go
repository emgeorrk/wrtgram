package wg_test

import (
	"testing"

	"github.com/emgeorrk/wrtgram/internal/repo/wg"
)

func TestParsePeers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		dump      string
		wantPeers int
		wantRx    uint64
		wantHS    bool
	}{
		{
			name: "wg: 4-field interface line and one peer",
			dump: "PRIV=\tPUB=\t51820\toff\n" +
				"PEER=\t(none)\t203.0.113.1:51820\t0.0.0.0/0\t1791377300\t100\t200\toff\n",
			wantPeers: 1, wantRx: 100, wantHS: true,
		},
		{
			name: "awg: long interface line, two peers, one never handshaked",
			dump: "PRIV=\tPUB=\t51820\toff\t4\t40\t70\t0\t0\t1\t2\t3\t4\n" +
				"P1=\t(none)\t203.0.113.1:8443\t0.0.0.0/1,128.0.0.0/1\t1791377300\t5\t6\t25\n" +
				"P2=\tPSK=\t(none)\t10.0.0.2/32\t0\t0\t0\toff\n",
			wantPeers: 2, wantRx: 5, wantHS: true,
		},
		{name: "empty (interface down)", dump: "", wantPeers: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			peers := wg.ParsePeers(tt.dump)
			if len(peers) != tt.wantPeers {
				t.Fatalf("ParsePeers() = %d peers, want %d", len(peers), tt.wantPeers)
			}

			if tt.wantPeers == 0 {
				return
			}

			if peers[0].RxBytes != tt.wantRx || peers[0].LastHandshake.IsZero() != !tt.wantHS {
				t.Errorf("peer 0 = %+v", peers[0])
			}

			if len(peers) > 1 && !peers[1].LastHandshake.IsZero() {
				t.Error("peer without handshake must have a zero time")
			}
		})
	}
}
