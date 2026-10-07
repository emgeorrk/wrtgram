// Package backup sends the configuration archive: /backup.
package backup

import (
	"context"

	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase/backup"
)

// Name is the module and UCI section name.
const Name = "backup"

// Module implements module.Module.
type Module struct {
	svc      *backup.Service
	hostname func(ctx context.Context) string
	detect   func(ctx context.Context) bool
}

// New creates the module.
func New(svc *backup.Service, hostname func(context.Context) string, detect func(context.Context) bool) *Module {
	return &Module{svc: svc, hostname: hostname, detect: detect}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module.
func (m *Module) Detect(ctx context.Context) bool { return m.detect(ctx) }

// Commands implements module.Module.
func (m *Module) Commands() []module.Command {
	return []module.Command{{Name: "backup", Description: "Send a backup of the router configuration", Handle: m.backup}}
}

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier { return nil }

func (m *Module) backup(ctx context.Context, _ module.Request) (module.Reply, error) {
	doc, cleanup, err := m.svc.Create(ctx, m.hostname(ctx))
	if err != nil {
		return module.Reply{}, err
	}

	// The controller reads the document while sending; release afterwards.
	context.AfterFunc(ctx, cleanup)

	return module.Reply{Document: doc}, nil
}
