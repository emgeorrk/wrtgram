// Package logread follows the system log with `logread -f` and parses its
// lines. The stream is restarted with backoff when logd goes away.
package logread

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

const (
	// layout is logread's timestamp: "Wed Oct  7 14:02:40 2026".
	layout       = "Mon Jan _2 15:04:05 2006"
	timestampLen = len("Wed Oct  7 14:02:40 2026")
	restartMin   = 2 * time.Second
	restartMax   = time.Minute
	backoffMul   = 2
	chanCap      = 64
)

var errFormat = errors.New("unrecognized log line")

// Tailer implements usecase.LogSource.
type Tailer struct {
	run   usecase.Runner
	clock usecase.Clock
	log   *slog.Logger
	loc   *time.Location
}

// New creates the adapter; loc is the router's local zone (logread prints local time).
func New(run usecase.Runner, clock usecase.Clock, loc *time.Location, log *slog.Logger) *Tailer {
	return &Tailer{run: run, clock: clock, log: log, loc: loc}
}

// Available reports whether logread exists.
func (t *Tailer) Available() bool { return t.run.LookPath("logread") }

// Tail implements usecase.LogSource. `logread -f -l 1` first replays the
// newest buffered line; that one is skipped so old events never fire again.
func (t *Tailer) Tail(ctx context.Context) (<-chan entity.LogLine, error) {
	out := make(chan entity.LogLine, chanCap)

	go func() {
		defer close(out)

		wait := restartMin

		for {
			began := t.clock.Now()

			if err := t.follow(ctx, out); err != nil && ctx.Err() == nil {
				t.log.Warn("logread stopped, restarting", "err", err, "in", wait)
			}

			if ctx.Err() != nil {
				return
			}

			if t.clock.Now().Sub(began) > restartMax {
				wait = restartMin
			}

			select {
			case <-ctx.Done():
				return
			case <-t.clock.After(wait):
			}

			wait = min(wait*backoffMul, restartMax)
		}
	}()

	return out, nil
}

func (t *Tailer) follow(ctx context.Context, out chan<- entity.LogLine) error {
	lines, err := t.run.Stream(ctx, "logread", "-f", "-l", "1")
	if err != nil {
		return err
	}

	first := true

	for raw := range lines {
		if first {
			first = false

			continue
		}

		line, err := Parse(raw, t.loc)
		if err != nil {
			continue
		}

		select {
		case out <- line:
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}

// Parse decodes "Wed Oct  7 14:02:40 2026 authpriv.notice dropbear[21949]: message".
func Parse(raw string, loc *time.Location) (entity.LogLine, error) {
	if len(raw) < timestampLen+1 {
		return entity.LogLine{}, fmt.Errorf("%w: %q", errFormat, raw)
	}

	at, err := time.ParseInLocation(layout, raw[:timestampLen], loc)
	if err != nil {
		return entity.LogLine{}, fmt.Errorf("%w: %q", errFormat, raw)
	}

	rest := strings.TrimSpace(raw[timestampLen:])

	facility, rest, ok := strings.Cut(rest, " ")
	if !ok {
		return entity.LogLine{}, fmt.Errorf("%w: %q", errFormat, raw)
	}

	tag, msg, ok := strings.Cut(rest, ": ")
	if !ok {
		return entity.LogLine{At: at, Facility: facility, Message: rest}, nil
	}

	if i := strings.IndexByte(tag, '['); i >= 0 {
		tag = tag[:i]
	}

	return entity.LogLine{At: at, Facility: facility, Tag: tag, Message: msg}, nil
}
