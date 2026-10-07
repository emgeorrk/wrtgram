// Package module defines what a feature module is and keeps the registry of
// the ones that are enabled and detected on this router.
package module

import (
	"context"

	"github.com/emgeorrk/wrtgram/internal/entity"
)

// Built-in commands served by the controller itself.
const (
	HelpCommand  = "help"
	StartCommand = "start"
)

// Module is a feature unit: it detects whether the router has what it needs,
// then contributes commands and background notifiers.
type Module interface {
	// Name is the UCI section name of the module.
	Name() string
	// Detect reports whether the required tools or ubus objects exist.
	Detect(ctx context.Context) bool
	Commands() []Command
	Notifiers() []Notifier
}

// HandlerFunc serves a command or a callback.
type HandlerFunc func(ctx context.Context, req Request) (Reply, error)

// Command is one bot command.
type Command struct {
	Handle      HandlerFunc // text command
	Callback    HandlerFunc // inline-button press with data "<name>:<payload>"; optional
	Name        string      // without the slash, ^[a-z0-9_]{1,32}$
	Description string      // menu and /help text
	Hidden      bool        // not listed in the menu and /help
	Confirm     bool        // wrap Handle with a Yes/No keyboard
}

// Request is the parsed input of a handler.
type Request struct {
	Payload      string // callback payload after "<name>:"
	Args         []string
	ChatID       int64
	MessageID    int
	FromCallback bool
}

// Reply is what a handler returns; the controller renders and sends it.
type Reply struct {
	Document *entity.Document  // send a file instead of text
	Text     string            // HTML
	Toast    string            // answerCallbackQuery text
	Keyboard [][]entity.Button // inline keyboard
	Edit     bool              // edit the originating message instead of sending a new one
}

// Notify delivers notifications to the configured chats without blocking.
type Notify interface {
	Message(ctx context.Context, msg entity.Message)
	Document(ctx context.Context, doc entity.Document)
}

// Notifier is a background producer of notifications. Run blocks until ctx is
// done; the supervisor restarts it with backoff when it returns an error.
type Notifier interface {
	Name() string
	Run(ctx context.Context, notify Notify) error
}
