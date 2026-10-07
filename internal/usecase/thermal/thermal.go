// Package thermal watches the sensors and raises overheat alerts with
// hysteresis: one alert above High, reminders while it stays hot, an
// all-clear once it drops below Normal.
package thermal

import (
	"context"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// Options tune the watcher.
type Options struct {
	High     float64
	Normal   float64
	Interval time.Duration
	Remind   time.Duration
}

// EventKind classifies thermal notifications.
type EventKind int

// Thermal events.
const (
	Overheat EventKind = iota // first crossing of High
	StillHot                  // reminder while above High
	Normal                    // back below Normal
)

// Event is one thermal notification.
type Event struct {
	Temps []entity.Temperature
	Max   float64
	Kind  EventKind
}

// Reader returns the current readings.
type Reader func() ([]entity.Temperature, error)

// Watcher polls the sensors.
type Watcher struct {
	read      Reader
	clock     usecase.Clock
	lastAlert time.Time
	opts      Options
	alerting  bool
}

// New creates the watcher.
func New(read Reader, clock usecase.Clock, opts Options) *Watcher {
	return &Watcher{read: read, clock: clock, opts: opts}
}

// Run polls until ctx is done.
func (w *Watcher) Run(ctx context.Context, emit func(Event)) error {
	for {
		if temps, err := w.read(); err == nil {
			if ev, ok := w.Step(temps, w.clock.Now()); ok {
				emit(ev)
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-w.clock.After(w.opts.Interval):
		}
	}
}

// Step applies one reading and reports whether a notification is due.
func (w *Watcher) Step(temps []entity.Temperature, now time.Time) (Event, bool) {
	maxT := 0.0
	for _, t := range temps {
		maxT = max(maxT, t.Celsius)
	}

	ev := Event{Temps: temps, Max: maxT}

	switch {
	case maxT > w.opts.High && !w.alerting:
		w.alerting, w.lastAlert = true, now
		ev.Kind = Overheat

		return ev, true
	case maxT > w.opts.High && now.Sub(w.lastAlert) >= w.opts.Remind:
		w.lastAlert = now
		ev.Kind = StillHot

		return ev, true
	case maxT < w.opts.Normal && w.alerting:
		w.alerting = false
		ev.Kind = Normal

		return ev, true
	}

	return Event{}, false
}
