// Package usecase implements the application rules. This file declares every
// port the usecases and modules depend on; adapters in internal/repo implement
// them and tests use the generated mocks.
package usecase

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=contracts.go -destination=mocks/usecase.go -package=mocks

import (
	"context"
	"io"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
)

// Runner executes external programs. Arguments are passed as argv, never
// through a shell; stdout is capped; a non-zero exit yields an error that
// carries stderr.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	RunInput(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)
	// Stream starts the program and returns its stdout line by line. The
	// channel closes when the program exits or ctx is done.
	Stream(ctx context.Context, name string, args ...string) (<-chan string, error)
	LookPath(name string) bool
}

// Ubus talks to the OpenWrt message bus (`ubus call`, `ubus list`).
type Ubus interface {
	// Call invokes object.method with args (marshaled to JSON, nil for none)
	// and decodes the reply into out.
	Call(ctx context.Context, object, method string, args, out any) error
	List(ctx context.Context, pattern string) ([]string, error)
}

// UCI reads and writes UCI configuration.
type UCI interface {
	Get(ctx context.Context, pkg string) (entity.UCIPackage, error)
	Set(ctx context.Context, pkg, section string, values map[string]string) error
	Commit(ctx context.Context, pkg string) error
}

// Sender is the outbound half of the Telegram transport.
type Sender interface {
	Me(ctx context.Context) (entity.BotInfo, error)
	SendMessage(ctx context.Context, chatID int64, msg entity.Message) (messageID int, err error)
	EditMessage(ctx context.Context, chatID int64, messageID int, msg entity.Message) error
	SendDocument(ctx context.Context, chatID int64, doc entity.Document) error
	AnswerCallback(ctx context.Context, callbackID, text string) error
	SetCommands(ctx context.Context, cmds []entity.BotCommand) error
	ChatAction(ctx context.Context, chatID int64, action string) error
}

// UpdateSource is the inbound half of the Telegram transport. The channel
// closes when ctx is done.
type UpdateSource interface {
	Updates(ctx context.Context) <-chan entity.Update
}

// Clock abstracts time for the periodic usecases (failover, thermal, backup).
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// Pinger probes reachability of target through a specific interface.
type Pinger interface {
	Ping(ctx context.Context, dev, target string) bool
}

// Router is the failover's view of the kernel routing table.
type Router interface {
	// DefaultDev returns the device owning the 0.0.0.0/1 route, "" when none.
	DefaultDev(ctx context.Context) (string, error)
	// SetDefault routes 0.0.0.0/1 and 128.0.0.0/1 through dev.
	SetDefault(ctx context.Context, dev string) error
	ClearDefault(ctx context.Context) error
	ReplaceNets(ctx context.Context, nets []string, dev string) error
	DeleteNets(ctx context.Context, nets []string) error
	// EnsureNTPDirect keeps the router's own NTP traffic on the WAN gateway;
	// it reports whether anything changed.
	EnsureNTPDirect(ctx context.Context, gw, dev string) (changed bool, err error)
}

// WAN exposes the upstream interface.
type WAN interface {
	Status(ctx context.Context) (entity.WANStatus, error)
}

// Tunnels lists configured VPN tunnels and reads their live state.
type Tunnels interface {
	List(ctx context.Context) ([]entity.TunnelRef, error)
	Status(ctx context.Context, ref entity.TunnelRef) (entity.Tunnel, error)
}

// StateStore persists small JSON documents by name.
type StateStore interface {
	Load(name string, v any) error
	Save(name string, v any) error
}

// Notifier delivers an out-of-band message to the configured chats.
type Notifier interface {
	Notify(ctx context.Context, msg entity.Message)
}
