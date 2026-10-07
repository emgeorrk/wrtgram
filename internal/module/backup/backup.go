// Package backup sends the configuration archive: /backup.
package backup

import (
	"context"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase/backup"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "backup"

// Module implements module.Module.
type Module struct {
	svc       *backup.Service
	scheduler *backup.Scheduler // nil when the weekly backup is off
	hostname  func(ctx context.Context) string
	detect    func(ctx context.Context) bool
}

// New creates the module.
func New(svc *backup.Service, scheduler *backup.Scheduler, hostname func(context.Context) string, detect func(context.Context) bool) *Module {
	return &Module{svc: svc, scheduler: scheduler, hostname: hostname, detect: detect}
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
func (m *Module) Notifiers() []module.Notifier {
	if m.scheduler == nil {
		return nil
	}

	return []module.Notifier{weekly{m: m}}
}

type weekly struct {
	m *Module
}

func (weekly) Name() string { return "weekly_backup" }

func (w weekly) Run(ctx context.Context, notify module.Notify) error {
	return w.m.scheduler.Run(ctx,
		func(doc entity.Document) { notify.Document(ctx, doc) },
		func(err error) {
			notify.Message(ctx, entity.Message{Text: "⚠️ Weekly backup failed: " + tgtext.Esc(err.Error())})
		})
}

func (m *Module) backup(ctx context.Context, _ module.Request) (module.Reply, error) {
	doc, cleanup, err := m.svc.Create(ctx, m.hostname(ctx))
	if err != nil {
		return module.Reply{}, err
	}

	// The controller reads the document while sending; release afterwards.
	context.AfterFunc(ctx, cleanup)

	return module.Reply{Document: doc}, nil
}
