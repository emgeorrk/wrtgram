// Package notify is the outbound notification queue. Producers never block
// on Telegram: messages are buffered and a single worker delivers them with
// retries, honoring rate limits and dropping messages older than the TTL.
package notify

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

const (
	defaultCap  = 100
	defaultTTL  = time.Hour
	attempts    = 3
	baseBackoff = 2 * time.Second
	maxBackoff  = 60 * time.Second
	backoffMul  = 4
)

type item struct {
	at  time.Time
	msg entity.Message
}

// Queue implements usecase.Notifier.
type Queue struct {
	sender  usecase.Sender
	clock   usecase.Clock
	log     *slog.Logger
	items   chan item
	chatIDs []int64
	ttl     time.Duration
}

// New creates a queue delivering to chatIDs.
func New(sender usecase.Sender, clock usecase.Clock, chatIDs []int64, log *slog.Logger) *Queue {
	return &Queue{
		sender:  sender,
		clock:   clock,
		log:     log,
		items:   make(chan item, defaultCap),
		chatIDs: chatIDs,
		ttl:     defaultTTL,
	}
}

// Notify enqueues a message; a full queue drops it with a warning.
func (q *Queue) Notify(_ context.Context, msg entity.Message) {
	select {
	case q.items <- item{at: q.clock.Now(), msg: msg}:
	default:
		q.log.Warn("notification queue full, dropping", "text", msg.Text)
	}
}

// Run delivers queued messages until ctx is done, then returns. Messages
// still queued at shutdown are delivered best-effort within grace.
func (q *Queue) Run(ctx context.Context, grace time.Duration) {
	for {
		select {
		case <-ctx.Done():
			q.drain(grace)

			return
		case it := <-q.items:
			q.deliver(ctx, it)
		}
	}
}

func (q *Queue) drain(grace time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()

	for {
		select {
		case it := <-q.items:
			q.deliver(ctx, it)
		default:
			return
		}
	}
}

func (q *Queue) deliver(ctx context.Context, it item) {
	if q.clock.Now().Sub(it.at) > q.ttl {
		q.log.Warn("notification expired before delivery", "text", it.msg.Text)

		return
	}

	for _, chat := range q.chatIDs {
		if err := q.sendWithRetry(ctx, chat, it.msg); err != nil {
			q.log.Error("notification not delivered", "chat", chat, "err", err)
		}
	}
}

func (q *Queue) sendWithRetry(ctx context.Context, chat int64, msg entity.Message) error {
	backoff := baseBackoff

	var err error

	for i := 0; i < attempts; i++ {
		if _, err = q.sender.SendMessage(ctx, chat, msg); err == nil {
			return nil
		}

		wait := backoff

		var rl *entity.RateLimitError
		if errors.As(err, &rl) {
			wait = rl.RetryAfter
		}

		select {
		case <-ctx.Done():
			return err
		case <-q.clock.After(wait):
		}

		backoff = min(backoff*backoffMul, maxBackoff)
	}

	return err
}
