package execx_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/emgeorrk/wrtgram/internal/repo/execx"
)

func TestRunnerRun(t *testing.T) {
	t.Parallel()

	r := execx.New()

	tests := []struct {
		name    string
		cmd     []string
		wantOut string
		wantErr error
	}{
		{name: "stdout", cmd: []string{"sh", "-c", "echo hi"}, wantOut: "hi\n"},
		{name: "exit code carries stderr", cmd: []string{"sh", "-c", "echo boom >&2; exit 3"}, wantErr: execx.ErrExit},
		{name: "missing program", cmd: []string{"definitely-not-a-program-xyz"}, wantErr: execx.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := r.Run(context.Background(), tt.cmd[0], tt.cmd[1:]...)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Run() err = %v, want %v", err, tt.wantErr)
			}

			if tt.wantErr == nil && string(out) != tt.wantOut {
				t.Errorf("Run() out = %q, want %q", out, tt.wantOut)
			}

			if errors.Is(tt.wantErr, execx.ErrExit) && !strings.Contains(err.Error(), "boom") {
				t.Errorf("stderr not included in error: %v", err)
			}
		})
	}
}

func TestRunnerTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := execx.New().Run(ctx, "sleep", "5")
	if !errors.Is(err, execx.ErrTimeout) {
		t.Fatalf("Run() err = %v, want ErrTimeout", err)
	}
}

func TestRunnerStream(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines, err := execx.New().Stream(ctx, "sh", "-c", "echo one; echo two")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var got []string
	for l := range lines {
		got = append(got, l)
	}

	if strings.Join(got, ",") != "one,two" {
		t.Errorf("Stream() lines = %v", got)
	}
}

func TestFake(t *testing.T) {
	t.Parallel()

	f := execx.NewFake().
		OnString("ubus call system board", `{"model":"x"}`).
		OnString("ubus call system board {\"a\":1}", `{"model":"specific"}`).
		OnError("ip route del", execx.ErrExit).
		OnStream("logread -f", []string{"l1", "l2"}).
		Path("awg", true).
		Path("wg", false)

	ctx := context.Background()

	out, err := f.Run(ctx, "ubus", "call", "system", "board", "{}")
	if err != nil || string(out) != `{"model":"x"}` {
		t.Fatalf("prefix match: %q %v", out, err)
	}

	out, _ = f.Run(ctx, "ubus", "call", "system", "board", `{"a":1}`)
	if string(out) != `{"model":"specific"}` {
		t.Errorf("longest prefix must win: %q", out)
	}

	if _, err := f.Run(ctx, "ip", "route", "del", "0.0.0.0/1"); !errors.Is(err, execx.ErrExit) {
		t.Errorf("OnError: %v", err)
	}

	if _, err := f.Run(ctx, "nothing"); !errors.Is(err, execx.ErrNoFixture) {
		t.Errorf("unknown command: %v", err)
	}

	if !f.LookPath("awg") || f.LookPath("wg") {
		t.Error("Path() not honoured")
	}

	sctx, cancel := context.WithCancel(ctx)
	lines, err := f.Stream(sctx, "logread", "-f")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if l := <-lines; l != "l1" {
		t.Errorf("first line = %q", l)
	}

	cancel()

	if calls := f.Calls(); len(calls) != 5 {
		t.Errorf("Calls() = %v", calls)
	}
}
