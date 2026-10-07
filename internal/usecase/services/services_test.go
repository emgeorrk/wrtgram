package services_test

import (
	"context"
	"testing"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/repo/execx"
	"github.com/emgeorrk/wrtgram/internal/repo/ubus"
	"github.com/emgeorrk/wrtgram/internal/usecase/services"
)

const list = `{"dnsmasq": {"instances": {"a": {"running": true, "pid": 1, "respawn": {"threshold": 3600}}}},
	"adblock": {"instances": {"adblock": {"running": false, "exit_code": 0}}},
	"cron": {"instances": {"i": {"running": false, "respawn": {"threshold": 3600}}}},
	"broken": {"instances": {"i": {"running": false, "exit_code": 2}}},
	"empty": {}}`

func TestListStates(t *testing.T) {
	t.Parallel()

	fake := execx.NewFake().OnString("ubus call service list", list)
	annotate := func(_ context.Context, name string) string {
		if name == "adblock" {
			return "enabled, 218 400 domains"
		}

		return ""
	}

	svc := services.New(ubus.New(fake), fake, []string{"adblock"}, annotate)

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := map[string]entity.ServiceState{
		"adblock": entity.ServiceDone, "broken": entity.ServiceFailed, "cron": entity.ServiceFailed,
		"dnsmasq": entity.ServiceRunning, "empty": entity.ServiceIdle,
	}

	if len(got) != len(want) {
		t.Fatalf("List() = %d services, want %d", len(got), len(want))
	}

	for i, s := range got {
		if st := s.State(); st != want[s.Name] {
			t.Errorf("%s: state = %v, want %v", s.Name, st, want[s.Name])
		}

		if s.Name == "adblock" && s.Detail == "" {
			t.Error("adblock must carry the annotation")
		}

		if i > 0 && got[i-1].Name > s.Name {
			t.Error("services must be sorted by name")
		}
	}

	if !svc.Allowed("adblock") || svc.Allowed("cron") {
		t.Error("whitelist not honoured")
	}
}
