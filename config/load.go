package config

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
)

// Environment overrides (highest precedence), mainly for development.
const (
	EnvToken    = "WRTGRAM_TOKEN"    //nolint:gosec // the name of an env var, not a credential
	EnvChatIDs  = "WRTGRAM_CHAT_IDS" // comma separated
	EnvProxy    = "WRTGRAM_PROXY"
	EnvLogLevel = "WRTGRAM_LOG_LEVEL"
)

// Load reads the package from src and applies env overrides. env may be nil.
func Load(ctx context.Context, src Source, env func(string) string) (Config, error) {
	pkg, err := src.Get(ctx, Package)
	if err != nil {
		return Config{}, fmt.Errorf("read uci package %s: %w", Package, err)
	}

	cfg, err := Parse(pkg)
	if err != nil {
		return Config{}, err
	}

	if env != nil {
		if err := cfg.applyEnv(env); err != nil {
			return Config{}, err
		}
	}

	return cfg, nil
}

// Defaults is the configuration of an absent package: everything enabled,
// no token, no chats.
func Defaults() Config {
	return Config{
		Modules: make(map[string]entity.UCISection),
		Notify:  make(map[string]entity.UCISection),
		Main:    Main{Workers: DefaultWorkers, LogLevel: slog.LevelInfo},
	}
}

// Parse builds a Config from a decoded UCI package.
func Parse(pkg entity.UCIPackage) (Config, error) {
	cfg := Defaults()

	if main, ok := pkg.Section(SectionMain); ok {
		if err := cfg.parseMain(main); err != nil {
			return Config{}, err
		}
	}

	for _, s := range pkg.OfType(TypeModule) {
		cfg.Modules[s.Name] = s
	}

	for _, s := range pkg.OfType(TypeNotify) {
		cfg.Notify[s.Name] = s
	}

	for _, s := range pkg.OfType(TypeCommand) {
		cmd, err := parseCommand(s)
		if err != nil {
			return Config{}, err
		}

		cfg.Commands = append(cfg.Commands, cmd)
	}

	return cfg, nil
}

func (c *Config) parseMain(s entity.UCISection) error {
	c.Main.Token = strings.TrimSpace(s.Opt("token", ""))
	c.Main.Proxy = strings.TrimSpace(s.Opt("proxy", ""))
	c.Main.WANInterface = strings.TrimSpace(s.Opt("wan_interface", ""))
	c.Main.Workers = s.Int("workers", DefaultWorkers)

	level, err := ParseLevel(s.Opt("log_level", "info"))
	if err != nil {
		return err
	}

	c.Main.LogLevel = level

	ids, err := parseChatIDs(s.List("chat_id"))
	if err != nil {
		return err
	}

	c.Main.ChatIDs = ids

	notify, err := parseChatIDs(s.List("notify_chat_id"))
	if err != nil {
		return err
	}

	c.Main.NotifyChatIDs = notify

	return nil
}

func (c *Config) applyEnv(env func(string) string) error {
	if v := env(EnvToken); v != "" {
		c.Main.Token = v
	}

	if v := env(EnvProxy); v != "" {
		c.Main.Proxy = v
	}

	if v := env(EnvChatIDs); v != "" {
		ids, err := parseChatIDs(strings.Split(v, ","))
		if err != nil {
			return err
		}

		c.Main.ChatIDs = ids
	}

	if v := env(EnvLogLevel); v != "" {
		level, err := ParseLevel(v)
		if err != nil {
			return err
		}

		c.Main.LogLevel = level
	}

	return nil
}

func parseCommand(s entity.UCISection) (CustomCommand, error) {
	cmd := CustomCommand{
		Name:        s.Name,
		Command:     s.Opt("command", ""),
		Description: s.Opt("description", "Custom command"),
		Timeout:     time.Duration(s.Int("timeout", int(DefaultCommandTimeout/time.Second))) * time.Second,
		Confirm:     s.Bool("confirm", false),
	}

	if strings.TrimSpace(cmd.Command) == "" {
		return CustomCommand{}, fmt.Errorf("%w: %s", errEmptyCommand, s.Name)
	}

	return cmd, nil
}

func parseChatIDs(raw []string) ([]int64, error) {
	ids := make([]int64, 0, len(raw))

	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}

		id, err := strconv.ParseInt(r, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", errBadChatID, r)
		}

		ids = append(ids, id)
	}

	return ids, nil
}

// ParseLevel converts a UCI log level name to slog.Level.
func ParseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}

	return slog.LevelInfo, fmt.Errorf("%w: %q", errBadLogLevel, name)
}
