// Package custom turns `config command` sections into bot commands.
package custom

import (
	"context"
	"strings"

	"github.com/emgeorrk/wrtgram/config"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase/custom"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "custom"

const maxOutput = 16 << 10

// Module implements module.Module.
type Module struct {
	exec *custom.Executor
	cmds []config.CustomCommand
}

// New creates the module.
func New(exec *custom.Executor, cmds []config.CustomCommand) *Module {
	return &Module{exec: exec, cmds: cmds}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module: at least one command configured.
func (m *Module) Detect(context.Context) bool { return len(m.cmds) > 0 }

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	out := make([]module.Command, 0, len(m.cmds))

	for _, c := range m.cmds {
		out = append(out, module.Command{Name: c.Name, Description: c.Description, Handle: m.handler(c), Confirm: c.Confirm})
	}

	return out
}

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier { return nil }

func (m *Module) handler(c config.CustomCommand) module.HandlerFunc {
	return func(ctx context.Context, req module.Request) (module.Reply, error) {
		if c.Confirm && !req.FromCallback {
			return module.Reply{Text: "Run " + tgtext.Code("/"+c.Name) + "?"}, nil
		}

		out, err := m.exec.Exec(ctx, c.Command, c.Timeout)
		if err != nil {
			return module.Reply{}, err
		}

		if len(out) > maxOutput {
			out = out[:maxOutput] + "\n…"
		}

		if strings.TrimSpace(out) == "" {
			out = "(no output)"
		}

		return module.Reply{Text: tgtext.Pre(out), Edit: c.Confirm}, nil
	}
}
