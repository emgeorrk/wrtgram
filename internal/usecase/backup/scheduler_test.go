package backup_test

import (
	"testing"
	"time"

	"github.com/emgeorrk/wrtgram/internal/usecase/backup"
)

func TestParseSchedule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		day, clock string
		wantDay    time.Weekday
		wantErr    bool
	}{
		{"sun", "03:30", time.Sunday, false},
		{"Monday", "23:59", time.Monday, false},
		{"xyz", "03:30", 0, true},
		{"sun", "25:00", 0, true},
		{"sun", "0330", 0, true},
	}

	for _, tt := range tests {
		s, err := backup.ParseSchedule(tt.day, tt.clock)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSchedule(%q, %q) err = %v", tt.day, tt.clock, err)
		}

		if err == nil && s.Day != tt.wantDay {
			t.Errorf("ParseSchedule(%q) day = %v, want %v", tt.day, s.Day, tt.wantDay)
		}
	}
}

func TestScheduleLastAndDue(t *testing.T) {
	t.Parallel()

	s := backup.Schedule{Day: time.Sunday, Hour: 3, Minute: 30}
	sched := backup.NewScheduler(nil, nil, nil, s, nil)

	// Wed 2026-10-07: the last Sunday 03:30 slot was 2026-10-04.
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	if got := s.Last(now); got != time.Date(2026, 10, 4, 3, 30, 0, 0, time.UTC) {
		t.Errorf("Last() = %v", got)
	}

	// Sunday 03:00 → the slot of today has not come yet: previous Sunday.
	sunEarly := time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC)
	if got := s.Last(sunEarly); got != time.Date(2026, 10, 4, 3, 30, 0, 0, time.UTC) {
		t.Errorf("Last(sunday early) = %v", got)
	}

	tests := []struct {
		name    string
		lastRun time.Time
		now     time.Time
		want    bool
	}{
		{"ran this week already", time.Date(2026, 10, 4, 3, 31, 0, 0, time.UTC), now, false},
		{"missed slot while off", time.Date(2026, 9, 27, 3, 31, 0, 0, time.UTC), now, true},
		{"slot passed just now", time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC), time.Date(2026, 10, 4, 3, 30, 0, 0, time.UTC), true},
		{"clock not synced", time.Time{}, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), false},
	}

	for _, tt := range tests {
		if got := sched.Due(tt.lastRun, tt.now); got != tt.want {
			t.Errorf("%s: Due() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
