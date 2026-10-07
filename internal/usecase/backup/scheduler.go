package backup

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// Schedule is a weekly slot.
type Schedule struct {
	Day    time.Weekday
	Hour   int
	Minute int
}

// ParseSchedule reads "sun" and "03:30".
func ParseSchedule(day, clock string) (Schedule, error) {
	days := map[string]time.Weekday{
		"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
		"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
	}

	name := strings.ToLower(strings.TrimSpace(day))

	d, ok := days[name[:min(dayAbbrev, len(name))]]
	if !ok {
		return Schedule{}, fmt.Errorf("%w: day %q", errSchedule, day)
	}

	h, m, ok := strings.Cut(strings.TrimSpace(clock), ":")
	if !ok {
		return Schedule{}, fmt.Errorf("%w: time %q", errSchedule, clock)
	}

	hour, err1 := strconv.Atoi(h)
	minute, err2 := strconv.Atoi(m)

	if err1 != nil || err2 != nil || hour < 0 || hour >= hoursPerDay || minute < 0 || minute >= minutesPerHour {
		return Schedule{}, fmt.Errorf("%w: time %q", errSchedule, clock)
	}

	return Schedule{Day: d, Hour: hour, Minute: minute}, nil
}

// Last returns the most recent occurrence of the slot at or before now.
func (s Schedule) Last(now time.Time) time.Time {
	slot := time.Date(now.Year(), now.Month(), now.Day(), s.Hour, s.Minute, 0, 0, now.Location())

	for slot.After(now) || slot.Weekday() != s.Day {
		slot = slot.AddDate(0, 0, -1)
	}

	return slot
}

const (
	dayAbbrev      = 3
	hoursPerDay    = 24
	minutesPerHour = 60
	tickEvery      = time.Minute
	saneYear       = 2024
	stateName      = "backup"
)

type schedState struct {
	LastRun time.Time `json:"last_run"`
}

// Scheduler runs the weekly backup. A missed slot (router off) fires at the
// next tick after it; the clock must look sane first (no RTC at boot).
type Scheduler struct {
	svc      *Service
	store    usecase.StateStore
	clock    usecase.Clock
	hostname func(ctx context.Context) string
	sched    Schedule
}

// NewScheduler creates the scheduler.
func NewScheduler(svc *Service, store usecase.StateStore, clock usecase.Clock, sched Schedule, hostname func(context.Context) string) *Scheduler {
	return &Scheduler{svc: svc, store: store, clock: clock, hostname: hostname, sched: sched}
}

// Run ticks every minute until ctx is done. send delivers the archive;
// fail reports an error.
func (s *Scheduler) Run(ctx context.Context, send func(entity.Document), fail func(error)) error {
	var st schedState

	if err := s.store.Load(stateName, &st); err != nil || st.LastRun.IsZero() {
		// First run: wait for the next slot instead of backing up right away.
		st.LastRun = s.clock.Now()

		if err := s.store.Save(stateName, st); err != nil {
			fail(err)
		}
	}

	for {
		if now := s.clock.Now(); s.Due(st.LastRun, now) {
			s.runOnce(ctx, send, fail)

			st.LastRun = now
			if err := s.store.Save(stateName, st); err != nil {
				fail(err)
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-s.clock.After(tickEvery):
		}
	}
}

// Due reports whether a slot passed since the last run.
func (s *Scheduler) Due(lastRun, now time.Time) bool {
	return now.Year() >= saneYear && s.sched.Last(now).After(lastRun)
}

func (s *Scheduler) runOnce(ctx context.Context, send func(entity.Document), fail func(error)) {
	doc, cleanup, err := s.svc.Create(ctx, s.hostname(ctx))
	if err != nil {
		fail(err)

		return
	}
	defer cleanup()

	doc.Caption = "🗄 Weekly " + strings.ToLower(doc.Caption[:1]) + doc.Caption[1:]

	send(*doc)
}
