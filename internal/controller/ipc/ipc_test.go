package ipc_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/emgeorrk/wrtgram/internal/controller/ipc"
	"github.com/emgeorrk/wrtgram/internal/entity"
)

type recorder struct {
	mu     sync.Mutex
	texts  []string
	events []entity.DHCPEvent
}

func (r *recorder) Notify(_ context.Context, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.texts = append(r.texts, text)
}

func (r *recorder) SendFile(_ context.Context, path, _ string) error {
	if path == "" {
		return errors.New("no path")
	}

	return nil
}

func (r *recorder) DHCPEvent(_ context.Context, ev entity.DHCPEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, ev)
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	sock := filepath.Join(t.TempDir(), "w.sock")
	rec := &recorder{}
	srv := ipc.NewServer(sock, rec, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = srv.Run(ctx) }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		err := ipc.Call(ctx, sock, ipc.Request{Op: ipc.OpNotify, Text: "hello"})
		if err == nil {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("daemon not reachable: %v", err)
		}

		time.Sleep(20 * time.Millisecond)
	}

	if err := ipc.Call(ctx, sock, ipc.Request{Op: ipc.OpEvent, Event: &entity.DHCPEvent{Action: "add", MAC: "aa:bb"}}); err != nil {
		t.Fatalf("event: %v", err)
	}

	if err := ipc.Call(ctx, sock, ipc.Request{Op: ipc.OpFile}); !errors.Is(err, ipc.ErrRejected) {
		t.Errorf("empty file path must be rejected, got %v", err)
	}

	if err := ipc.Call(ctx, sock, ipc.Request{Op: "nope"}); !errors.Is(err, ipc.ErrRejected) {
		t.Errorf("unknown op must be rejected, got %v", err)
	}

	if err := ipc.Call(ctx, filepath.Join(t.TempDir(), "missing.sock"), ipc.Request{Op: ipc.OpNotify, Text: "x"}); !errors.Is(err, ipc.ErrNoDaemon) {
		t.Errorf("missing socket must be ErrNoDaemon, got %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.texts) != 1 || rec.texts[0] != "hello" || len(rec.events) != 1 || rec.events[0].MAC != "aa:bb" {
		t.Errorf("recorded = %v %v", rec.texts, rec.events)
	}
}
