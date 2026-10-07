package iproute_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/emgeorrk/wrtgram/internal/repo/execx"
	"github.com/emgeorrk/wrtgram/internal/repo/iproute"
)

func TestDevOf(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"0.0.0.0/1 dev awg0 scope link":          "awg0",
		"0.0.0.0/1 via 10.0.0.1 dev eth0 metric": "eth0",
		"":                                       "",
		"dev":                                    "",
	}

	for in, want := range tests {
		if got := iproute.DevOf(in); got != want {
			t.Errorf("DevOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnsureNTPDirect(t *testing.T) {
	t.Parallel()

	fake := execx.NewFake().
		OnString("ip rule show pref 29990", "").
		OnString("ip rule add", "").
		OnString("ip route replace default", "").
		OnString("/etc/init.d/sysntpd restart", "")

	r := iproute.NewRouter(fake, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	changed, err := r.EnsureNTPDirect(ctx, "10.0.0.1", "wan")
	if err != nil || !changed {
		t.Fatalf("first call: changed=%v err=%v", changed, err)
	}

	changed, err = r.EnsureNTPDirect(ctx, "10.0.0.1", "wan")
	if err != nil || changed {
		t.Fatalf("same gateway must be a no-op: changed=%v err=%v", changed, err)
	}

	calls := strings.Join(fake.Calls(), "\n")
	for _, want := range []string{
		"ip rule add pref 29990 iif lo ipproto udp dport 123 lookup 123",
		"ip route replace default via 10.0.0.1 dev wan table 123",
		"/etc/init.d/sysntpd restart",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("missing call %q in:\n%s", want, calls)
		}
	}

	if strings.Count(calls, "sysntpd restart") != 1 {
		t.Errorf("sysntpd must restart once:\n%s", calls)
	}
}
