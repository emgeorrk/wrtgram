package throttle_test

import (
	"testing"
	"time"

	"github.com/emgeorrk/wrtgram/pkg/throttle"
)

func TestAllow(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	window := time.Hour

	tests := []struct {
		name  string
		steps []struct {
			key  string
			at   time.Duration
			want bool
		}
	}{
		{
			name: "first passes, second within window blocked, after window passes",
			steps: []struct {
				key  string
				at   time.Duration
				want bool
			}{
				{"a", 0, true},
				{"a", 30 * time.Minute, false},
				{"a", time.Hour, true},
			},
		},
		{
			name: "keys are independent",
			steps: []struct {
				key  string
				at   time.Duration
				want bool
			}{
				{"a", 0, true},
				{"b", 0, true},
				{"a", time.Minute, false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := throttle.New()

			for i, s := range tt.steps {
				if got := l.Allow(s.key, window, base.Add(s.at)); got != s.want {
					t.Fatalf("step %d: Allow(%q) = %v, want %v", i, s.key, got, s.want)
				}
			}
		})
	}
}

func TestReset(t *testing.T) {
	t.Parallel()

	l := throttle.New()
	now := time.Now()

	if !l.Allow("k", time.Hour, now) {
		t.Fatal("first Allow must pass")
	}

	l.Reset("k")

	if !l.Allow("k", time.Hour, now) {
		t.Fatal("Allow after Reset must pass")
	}
}
