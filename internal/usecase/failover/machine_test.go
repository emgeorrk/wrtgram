package failover_test

import (
	"testing"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase/failover"
)

var rules = failover.Rules{Tunnels: []string{"awg0", "awg1"}, Grace: 5 * time.Minute, Interval: 30 * time.Second}

type probe struct {
	health map[string]bool
	at     time.Duration // offset from t0
}

func h(p, b bool) map[string]bool { return map[string]bool{"awg0": p, "awg1": b} }

func TestStep(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		manualOff bool
		probes    []probe
		wantDev   string
		wantMode  entity.FailoverMode
		wantSvc   string
		wantKinds []entity.FailoverEventKind
		wantDown  []time.Duration // per event; checked when set
	}{
		{
			name:     "all healthy stays on primary, no events",
			probes:   []probe{{h(true, true), 0}, {h(true, true), 30 * time.Second}},
			wantDev:  "awg0", wantMode: entity.FailoverPrimary, wantSvc: "awg0",
		},
		{
			name:     "primary down less than grace: stay",
			probes:   []probe{{h(false, true), 0}, {h(false, true), 2 * time.Minute}},
			wantDev:  "awg0", wantMode: entity.FailoverPrimary, wantSvc: "awg1",
		},
		{
			name:      "primary down past grace: backup",
			probes:    []probe{{h(false, true), 0}, {h(false, true), 5 * time.Minute}},
			wantDev:   "awg1", wantMode: entity.FailoverBackup, wantSvc: "awg1",
			wantKinds: []entity.FailoverEventKind{entity.FailoverSwitched},
		},
		{
			name:      "primary returns: back immediately",
			probes:    []probe{{h(false, true), 0}, {h(false, true), 5 * time.Minute}, {h(true, true), 6 * time.Minute}},
			wantDev:   "awg0", wantMode: entity.FailoverPrimary, wantSvc: "awg0",
			wantKinds: []entity.FailoverEventKind{entity.FailoverSwitched, entity.FailoverPrimaryBack},
			wantDown:  []time.Duration{5 * time.Minute, 6 * time.Minute},
		},
		{
			// Since is the last route change (hours ago), the outage is minutes.
			name:      "long on primary, short outage: downtime counts from the outage",
			probes:    []probe{{h(true, true), 0}, {h(false, true), 6 * time.Hour}, {h(false, true), 6*time.Hour + 5*time.Minute}, {h(true, true), 6*time.Hour + 12*time.Minute}},
			wantDev:   "awg0", wantMode: entity.FailoverPrimary, wantSvc: "awg0",
			wantKinds: []entity.FailoverEventKind{entity.FailoverSwitched, entity.FailoverPrimaryBack},
			wantDown:  []time.Duration{5 * time.Minute, 12 * time.Minute},
		},
		{
			name:      "long on primary, both die, backup returns: outage, then time direct",
			probes:    []probe{{h(true, true), 0}, {h(false, false), 2 * time.Hour}, {h(false, false), 2*time.Hour + 5*time.Minute}, {h(false, true), 2*time.Hour + 8*time.Minute}},
			wantDev:   "awg1", wantMode: entity.FailoverBackup, wantSvc: "awg1",
			wantKinds: []entity.FailoverEventKind{entity.FailoverAllDown, entity.FailoverTunnelUp},
			wantDown:  []time.Duration{5 * time.Minute, 3 * time.Minute},
		},
		{
			name:      "both down past grace: direct",
			probes:    []probe{{h(false, false), 0}, {h(false, false), 5 * time.Minute}},
			wantDev:   "", wantMode: entity.FailoverDirect, wantSvc: "",
			wantKinds: []entity.FailoverEventKind{entity.FailoverAllDown},
		},
		{
			name:      "backup comes up while direct",
			probes:    []probe{{h(false, false), 0}, {h(false, false), 5 * time.Minute}, {h(false, true), 6 * time.Minute}},
			wantDev:   "awg1", wantMode: entity.FailoverBackup, wantSvc: "awg1",
			wantKinds: []entity.FailoverEventKind{entity.FailoverAllDown, entity.FailoverTunnelUp},
		},
		{
			name:      "on backup and it dies: grace then direct",
			probes:    []probe{{h(false, true), 0}, {h(false, true), 5 * time.Minute}, {h(false, false), 6 * time.Minute}, {h(false, false), 12 * time.Minute}},
			wantDev:   "", wantMode: entity.FailoverDirect,
			wantKinds: []entity.FailoverEventKind{entity.FailoverSwitched, entity.FailoverAllDown},
		},
		{
			name:      "manual off: direct without events, service nets on a live tunnel",
			manualOff: true,
			probes:    []probe{{h(true, true), 0}},
			wantDev:   "", wantMode: entity.FailoverDirect, wantSvc: "awg0",
		},
		{
			name:      "flap: primary down, up, down again restarts the grace",
			probes:    []probe{{h(false, true), 0}, {h(true, true), 4 * time.Minute}, {h(false, true), 5 * time.Minute}, {h(false, true), 9 * time.Minute}},
			wantDev:   "awg0", wantMode: entity.FailoverPrimary, wantSvc: "awg1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := failover.Initial(rules, tt.manualOff, t0)

			var (
				kinds []entity.FailoverEventKind
				downs []time.Duration
			)

			for _, p := range tt.probes {
				var events []entity.FailoverEvent

				st, events = failover.Step(st, p.health, t0.Add(p.at), rules)
				for _, ev := range events {
					kinds = append(kinds, ev.Kind)
					downs = append(downs, ev.Downtime)
				}
			}

			if st.Dev != tt.wantDev || st.Mode != tt.wantMode || st.ServiceDev != tt.wantSvc {
				t.Errorf("state = dev %q mode %s svc %q, want dev %q mode %s svc %q", st.Dev, st.Mode, st.ServiceDev, tt.wantDev, tt.wantMode, tt.wantSvc)
			}

			if len(kinds) != len(tt.wantKinds) {
				t.Fatalf("events = %v, want %v", kinds, tt.wantKinds)
			}

			for i := range kinds {
				if kinds[i] != tt.wantKinds[i] {
					t.Errorf("event %d = %v, want %v", i, kinds[i], tt.wantKinds[i])
				}

				if tt.wantDown != nil && downs[i] != tt.wantDown[i] {
					t.Errorf("event %d downtime = %v, want %v", i, downs[i], tt.wantDown[i])
				}
			}
		})
	}
}
