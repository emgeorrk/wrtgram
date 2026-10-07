// Package notify is the outbound notification queue. Producers never block
// on Telegram: messages are buffered and a single worker delivers them with
// retries, honoring rate limits and dropping messages older than the TTL.
package notify

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	// sendTimeout bounds one delivery attempt; an attempt in flight at
	// shutdown is allowed to finish (procd waits term_timeout = 10 s).
	sendTimeout = 8 * time.Second
)

type item struct {
	at   time.Time
	doc  *entity.Document // when set, a file is sent instead of msg
	data []byte           // the file content, read once so every chat gets it
	msg  entity.Message
}

const maxDocument = 16 << 20

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
	q.enqueue(item{at: q.clock.Now(), msg: msg})
}

// Message implements module.Notify.
func (q *Queue) Message(ctx context.Context, msg entity.Message) { q.Notify(ctx, msg) }

// NotifyDocument enqueues a file. Its content is read now, so the caller may
// release the source right after the call.
func (q *Queue) NotifyDocument(_ context.Context, doc entity.Document) {
	data, err := io.ReadAll(io.LimitReader(doc.Data, maxDocument+1))
	if err != nil || len(data) > maxDocument {
		q.log.Warn("document not queued", "name", doc.Name, "err", err, "size", len(data))

		return
	}

	q.enqueue(item{at: q.clock.Now(), doc: &doc, data: data})
}

// Document implements module.Notify.
func (q *Queue) Document(ctx context.Context, doc entity.Document) { q.NotifyDocument(ctx, doc) }

func (q *Queue) enqueue(it item) {
	select {
	case q.items <- it:
	default:
		q.log.Warn("notification queue full, dropping", "text", it.msg.Text)
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
		if err := q.sendWithRetry(ctx, chat, it); err != nil {
			q.log.Error("notification not delivered", "chat", chat, "err", err)
		}
	}
}

func (q *Queue) send(ctx context.Context, chat int64, it item) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTimeout)
	defer cancel()

	if it.doc == nil {
		_, err := q.sender.SendMessage(ctx, chat, it.msg)

		return err
	}

	doc := *it.doc
	doc.Data = bytes.NewReader(it.data)

	return q.sender.SendDocument(ctx, chat, doc)
}

func (q *Queue) sendWithRetry(ctx context.Context, chat int64, it item) error {
	backoff := baseBackoff

	var err error

	for i := 0; i < attempts; i++ {
		if err = q.send(ctx, chat, it); err == nil {
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
