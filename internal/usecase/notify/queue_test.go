package notify_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase/mocks"
	"github.com/emgeorrk/wrtgram/internal/usecase/notify"
	"github.com/emgeorrk/wrtgram/pkg/logger"
)

// fakeClock advances instantly on After so retries do not sleep.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()

	ch := make(chan time.Time, 1)
	ch <- c.now

	return ch
}

var errBoom = errors.New("boom")

func TestQueueRetries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		results   []error
		wantCalls int
	}{
		{name: "first try", results: []error{nil}, wantCalls: 1},
		{name: "rate limited then ok", results: []error{&entity.RateLimitError{RetryAfter: time.Second}, nil}, wantCalls: 2},
		{name: "gives up after three", results: []error{errBoom, errBoom, errBoom}, wantCalls: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			sender := mocks.NewMockSender(ctrl)

			done := make(chan struct{})
			calls := 0

			sender.EXPECT().SendMessage(gomock.Any(), int64(42), gomock.Any()).
				DoAndReturn(func(context.Context, int64, entity.Message) (int, error) {
					err := tt.results[calls]
					calls++

					if calls == tt.wantCalls {
						close(done)
					}

					return 1, err
				}).Times(tt.wantCalls)

			q := notify.New(sender, &fakeClock{now: time.Now()}, []int64{42}, logger.New(nopWriter{}, logger.Options{}))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go q.Run(ctx, 0)

			q.Notify(ctx, entity.Message{Text: "hi"})

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("timeout waiting for sends")
			}
		})
	}
}

func TestQueueExpired(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sender := mocks.NewMockSender(ctrl) // no SendMessage expected

	clock := &fakeClock{now: time.Now()}
	q := notify.New(sender, clock, []int64{1}, logger.New(nopWriter{}, logger.Options{}))

	ctx, cancel := context.WithCancel(context.Background())

	q.Notify(ctx, entity.Message{Text: "old"})
	clock.After(2 * time.Hour)

	cancel()
	q.Run(ctx, time.Second) // drains: the message is older than the TTL and must be dropped
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
