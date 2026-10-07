// Package config turns the UCI package `wrtgram` into a typed configuration.
// The daemon reads it through ubus; development and tests feed the same
// decoder from a JSON file. Environment variables override the main section.
package config

import (
	"context"
	"log/slog"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
)

// Package is the UCI package name.
const Package = "wrtgram"

// Section types and names inside the package.
const (
	TypeMain    = "main"
	TypeModule  = "module"
	TypeNotify  = "notify"
	TypeCommand = "command"
	SectionMain = "main"
)

// Defaults.
const (
	DefaultWorkers        = 2
	DefaultCommandTimeout = 30 * time.Second
	optEnabled            = "enabled"
)

// TelegramNets are Telegram's published address ranges: the default
// service networks the failover keeps on a live tunnel so the bot works even
// where Telegram is blocked.
//
//nolint:gochecknoglobals // immutable default list
var TelegramNets = []string{
	"149.154.160.0/20", "91.108.4.0/22", "91.108.8.0/22", "91.108.12.0/22",
	"91.108.16.0/22", "91.108.20.0/22", "91.108.56.0/22", "185.76.151.0/24",
}

// Source provides the raw UCI package (ubus in production, a file in dev).
type Source interface {
	Get(ctx context.Context, pkg string) (entity.UCIPackage, error)
}

// Config is the typed configuration.
type Config struct {
	Modules  map[string]entity.UCISection // by module name
	Notify   map[string]entity.UCISection // by notification name
	Commands []CustomCommand
	Main     Main
}

// Main is the `config main 'main'` section.
type Main struct {
	Token         string
	Proxy         string // http://, https://, socks5://, socks5h:// or ""
	WANInterface  string // "" = auto
	ChatIDs       []int64
	NotifyChatIDs []int64 // defaults to ChatIDs
	LogLevel      slog.Level
	Workers       int
}

// CustomCommand is a user-defined shell command exposed as /name.
type CustomCommand struct {
	Name        string
	Command     string
	Description string
	Timeout     time.Duration
	Confirm     bool
}

// Module returns the section of a module; a missing section is an empty,
// enabled one so every detected module works out of the box.
func (c Config) Module(name string) entity.UCISection {
	if s, ok := c.Modules[name]; ok {
		return s
	}

	return entity.UCISection{Name: name, Type: TypeModule}
}

// ModuleEnabled reports whether a module is switched on (default true).
func (c Config) ModuleEnabled(name string) bool {
	return c.Module(name).Bool(optEnabled, true)
}

// Notification returns the section of a notification rule (empty = defaults).
func (c Config) Notification(name string) entity.UCISection {
	if s, ok := c.Notify[name]; ok {
		return s
	}

	return entity.UCISection{Name: name, Type: TypeNotify}
}

// NotificationEnabled reports whether a notification rule is on (default true).
func (c Config) NotificationEnabled(name string) bool {
	return c.Notification(name).Bool(optEnabled, true)
}

// Allowed reports whether a chat may talk to the bot.
func (c Config) Allowed(chatID int64) bool {
	for _, id := range c.Main.ChatIDs {
		if id == chatID {
			return true
		}
	}

	return false
}
