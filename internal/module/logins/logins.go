// Package logins reports SSH and LuCI authentication events.
package logins

import (
	"context"
	"fmt"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase/logwatch"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// Name is the module and UCI section name.
const Name = "logins"

// Module implements module.Module.
type Module struct {
	watcher *logwatch.Watcher
	detect  func(ctx context.Context) bool
}

// New creates the module; watcher may be nil when the notification is disabled.
func New(watcher *logwatch.Watcher, detect func(context.Context) bool) *Module {
	return &Module{watcher: watcher, detect: detect}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Detect implements module.Module.
func (m *Module) Detect(ctx context.Context) bool { return m.watcher != nil && m.detect(ctx) }

// Commands implements module.Module: none yet.
func (m *Module) Commands() []module.Command { return nil }

// Notifiers implements module.Module.
func (m *Module) Notifiers() []module.Notifier { return []module.Notifier{notifier{m: m}} }

type notifier struct {
	m *Module
}

func (notifier) Name() string { return Name }

func (n notifier) Run(ctx context.Context, notify module.Notify) error {
	return n.m.watcher.Run(ctx, func(ev entity.LoginEvent) {
		notify.Message(ctx, entity.Message{Text: Render(ev)})
	})
}

// Render turns a login event into a message.
func Render(ev entity.LoginEvent) string {
	ip := tgtext.Code(ev.IP)

	switch {
	case ev.Kind == entity.LoginLuCI && ev.Success:
		return fmt.Sprintf("🔑 Web interface (LuCI) login as %s from %s", tgtext.Esc(ev.User), ip)
	case ev.Kind == entity.LoginLuCI:
		return fmt.Sprintf("⛔️ Failed LuCI login as %s from %s", tgtext.Esc(ev.User), ip)
	case ev.Success:
		return fmt.Sprintf("🖥 SSH login as %s by %s from %s", tgtext.Esc(ev.User), ev.Method, ip)
	}

	return fmt.Sprintf("⛔️ Failed SSH login attempt from %s", ip)
}
