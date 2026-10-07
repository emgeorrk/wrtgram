package module

import (
	"context"
	"fmt"
	"log/slog"
)

const maxCommandName = 32

// Registry holds the active modules and routes command names to handlers.
type Registry struct {
	commands map[string]Command
	owner    map[string]string // command → module
	log      *slog.Logger
	modules  []Module
	order    []string
}

// NewRegistry creates an empty registry.
func NewRegistry(log *slog.Logger) *Registry {
	return &Registry{commands: make(map[string]Command), owner: make(map[string]string), log: log}
}

// Add activates m when enabled and detected. Disabled or undetected modules
// are logged and skipped — never an error, so a missing tool hides a module
// instead of stopping the bot.
func (r *Registry) Add(ctx context.Context, m Module, enabled bool) error {
	if !enabled {
		r.log.Debug("module disabled", "module", m.Name())

		return nil
	}

	if !m.Detect(ctx) {
		r.log.Info("module not available on this router", "module", m.Name())

		return nil
	}

	for _, c := range m.Commands() {
		if err := r.register(c); err != nil {
			return fmt.Errorf("module %s: %w", m.Name(), err)
		}

		r.owner[c.Name] = m.Name()
	}

	r.modules = append(r.modules, m)
	r.log.Info("module active", "module", m.Name(), "commands", len(m.Commands()))

	return nil
}

func (r *Registry) register(c Command) error {
	if !ValidCommandName(c.Name) {
		return fmt.Errorf("%w: %q", errBadCommandName, c.Name)
	}

	if _, dup := r.commands[c.Name]; dup {
		return fmt.Errorf("%w: /%s", errDuplicateCommand, c.Name)
	}

	r.commands[c.Name] = c
	r.order = append(r.order, c.Name)

	return nil
}

// Lookup finds a command by name.
func (r *Registry) Lookup(name string) (Command, bool) {
	c, ok := r.commands[name]

	return c, ok
}

// Owner returns the module that registered a command ("" when unknown).
func (r *Registry) Owner(name string) string { return r.owner[name] }

// Has reports whether a command name is taken.
func (r *Registry) Has(name string) bool {
	_, ok := r.commands[name]

	return ok
}

// Commands returns every command in registration order.
func (r *Registry) Commands() []Command {
	out := make([]Command, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.commands[n])
	}

	return out
}

// Modules returns the active modules in registration order.
func (r *Registry) Modules() []Module { return r.modules }

// Notifiers collects the notifiers of every active module.
func (r *Registry) Notifiers() []Notifier {
	var out []Notifier
	for _, m := range r.modules {
		out = append(out, m.Notifiers()...)
	}

	return out
}

// ValidCommandName reports whether name matches ^[a-z0-9_]{1,32}$ (Telegram's rule).
func ValidCommandName(name string) bool {
	if name == "" || len(name) > maxCommandName {
		return false
	}

	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}

	return true
}
