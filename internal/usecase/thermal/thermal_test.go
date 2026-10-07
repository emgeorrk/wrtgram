package thermal_test

import (
	"testing"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase/thermal"
)

func TestStep(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	opts := thermal.Options{High: 85, Normal: 75, Interval: time.Minute, Remind: time.Hour}

	steps := []struct {
		at       time.Duration
		cpu      float64
		wantKind thermal.EventKind
		wantOK   bool
	}{
		{0, 70, 0, false},
		{1 * time.Minute, 86, thermal.Overheat, true},
		{2 * time.Minute, 90, 0, false}, // still hot, reminder not due
		{30 * time.Minute, 80, 0, false}, // between Normal and High: nothing
		{61 * time.Minute, 88, thermal.StillHot, true},
		{62 * time.Minute, 74, thermal.Normal, true},
		{63 * time.Minute, 74, 0, false},
	}

	w := thermal.New(nil, nil, opts)

	for i, s := range steps {
		ev, ok := w.Step([]entity.Temperature{{Sensor: "cpu", Celsius: s.cpu}, {Sensor: "wifi", Celsius: 50}}, t0.Add(s.at))
		if ok != s.wantOK || (ok && ev.Kind != s.wantKind) {
			t.Fatalf("step %d: got ok=%v kind=%v, want ok=%v kind=%v", i, ok, ev.Kind, s.wantOK, s.wantKind)
		}

		if ok && ev.Max != s.cpu {
			t.Errorf("step %d: max = %v, want %v", i, ev.Max, s.cpu)
		}
	}
}
