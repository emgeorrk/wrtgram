package uci_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/emgeorrk/wrtgram/internal/repo/execx"
	"github.com/emgeorrk/wrtgram/internal/repo/ubus"
	"github.com/emgeorrk/wrtgram/internal/repo/uci"
)

const reply = `{"values": {
	"main": {".anonymous": false, ".type": "main", ".name": "main", ".index": 0, "token": "t", "chat_id": ["1", "-2"]},
	"cfg0": {".anonymous": true, ".type": "command", ".name": "cfg0", ".index": 2, "command": "ls"},
	"speed": {".anonymous": false, ".type": "command", ".name": "speed", ".index": 1, "command": "x", "confirm": "1"}
}}`

func TestDecode(t *testing.T) {
	t.Parallel()

	var r struct {
		Values map[string]map[string]json.RawMessage `json:"values"`
	}

	if err := json.Unmarshal([]byte(reply), &r); err != nil {
		t.Fatal(err)
	}

	pkg, err := uci.Decode("wrtgram", r.Values)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	main, ok := pkg.Section("main")
	if !ok || main.Type != "main" || main.Opt("token", "") != "t" {
		t.Errorf("main section: %+v", main)
	}

	if got := main.List("chat_id"); len(got) != 2 || got[1] != "-2" {
		t.Errorf("list: %v", got)
	}

	cmds := pkg.OfType("command")
	if len(cmds) != 2 || cmds[0].Name != "speed" || cmds[1].Name != "cfg0" {
		t.Errorf("OfType order by index: %+v", cmds)
	}

	if !cmds[0].Bool("confirm", false) {
		t.Error("confirm must be true")
	}
}

func TestClientGetSetCommit(t *testing.T) {
	t.Parallel()

	fake := execx.NewFake().
		OnString("ubus call uci get", reply).
		OnString("ubus call uci set", "").
		OnString("ubus call uci commit", "")

	c := uci.New(ubus.New(fake))
	ctx := context.Background()

	pkg, err := c.Get(ctx, "wrtgram")
	if err != nil || pkg.Name != "wrtgram" || len(pkg.Sections) != 3 {
		t.Fatalf("Get: %+v %v", pkg, err)
	}

	if err := c.Set(ctx, "wrtgram", "failover", map[string]any{"manual_off": "1"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := c.Commit(ctx, "wrtgram"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	calls := fake.Calls()
	if len(calls) != 3 || calls[1] != `ubus call uci set {"config":"wrtgram","section":"failover","values":{"manual_off":"1"}}` {
		t.Errorf("calls: %q", calls)
	}
}
