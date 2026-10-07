package module_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/emgeorrk/wrtgram/internal/module"
)

type fakeModule struct {
	name   string
	cmds   []module.Command
	detect bool
}

func (m fakeModule) Name() string                 { return m.name }
func (m fakeModule) Detect(context.Context) bool  { return m.detect }
func (m fakeModule) Commands() []module.Command   { return m.cmds }
func (m fakeModule) Notifiers() []module.Notifier { return nil }

func cmd(name string) module.Command { return module.Command{Name: name, Description: name} }

func TestRegistryAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		modules  []fakeModule
		enabled  []bool
		wantErr  bool
		wantCmds []string
	}{
		{
			name:     "detected modules register in order",
			modules:  []fakeModule{{name: "a", detect: true, cmds: []module.Command{cmd("x")}}, {name: "b", detect: true, cmds: []module.Command{cmd("y"), cmd("z")}}},
			enabled:  []bool{true, true},
			wantCmds: []string{"x", "y", "z"},
		},
		{
			name:     "undetected and disabled modules are skipped silently",
			modules:  []fakeModule{{name: "a", detect: false, cmds: []module.Command{cmd("x")}}, {name: "b", detect: true, cmds: []module.Command{cmd("y")}}},
			enabled:  []bool{true, false},
			wantCmds: nil,
		},
		{
			name:    "duplicate command is an error",
			modules: []fakeModule{{name: "a", detect: true, cmds: []module.Command{cmd("x")}}, {name: "b", detect: true, cmds: []module.Command{cmd("x")}}},
			enabled: []bool{true, true},
			wantErr: true,
		},
		{
			name:    "invalid command name is an error",
			modules: []fakeModule{{name: "a", detect: true, cmds: []module.Command{cmd("Bad-Name")}}},
			enabled: []bool{true},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := module.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))

			var err error
			for i, m := range tt.modules {
				if err = r.Add(context.Background(), m, tt.enabled[i]); err != nil {
					break
				}
			}

			if (err != nil) != tt.wantErr {
				t.Fatalf("Add() err = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			got := r.Commands()
			if len(got) != len(tt.wantCmds) {
				t.Fatalf("Commands() = %d, want %d", len(got), len(tt.wantCmds))
			}

			for i, c := range got {
				if c.Name != tt.wantCmds[i] {
					t.Errorf("command %d = %q, want %q", i, c.Name, tt.wantCmds[i])
				}

				if _, ok := r.Lookup(c.Name); !ok {
					t.Errorf("Lookup(%q) failed", c.Name)
				}
			}
		})
	}
}

func TestValidCommandName(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"status": true, "vpn_off": true, "a1": true, "": false, "Status": false, "a-b": false,
		"abcdefghijklmnopqrstuvwxyz0123456": false, // 33 chars
	}

	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			if got := module.ValidCommandName(in); got != want {
				t.Errorf("ValidCommandName(%q) = %v, want %v", in, got, want)
			}
		})
	}
}
