package config_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/emgeorrk/wrtgram/config"
	"github.com/emgeorrk/wrtgram/internal/entity"
)

const goodToken = "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef-_12345"

type staticSource struct{ pkg entity.UCIPackage }

func (s staticSource) Get(context.Context, string) (entity.UCIPackage, error) { return s.pkg, nil }

func samplePackage() entity.UCIPackage {
	return entity.UCIPackage{
		Name: config.Package,
		Sections: map[string]entity.UCISection{
			"main": {
				Name: "main", Type: "main", Index: 0,
				Options: map[string]string{"token": goodToken, "log_level": "debug", "workers": "3"},
				Lists:   map[string][]string{"chat_id": {"42", "-100500"}},
			},
			"failover": {
				Name: "failover", Type: "module", Index: 1,
				Options: map[string]string{"enabled": "0", "grace": "60"},
				Lists:   map[string][]string{"tunnel": {"awg0", "awg1"}},
			},
			"logins": {
				Name: "logins", Type: "notify", Index: 2,
				Options: map[string]string{"success_window": "10"},
			},
			"speed": {
				Name: "speed", Type: "command", Index: 3,
				Options: map[string]string{"command": "speedtest", "timeout": "90", "confirm": "1"},
			},
		},
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(context.Background(), staticSource{samplePackage()}, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Main.Token != goodToken || cfg.Main.Workers != 3 || cfg.Main.LogLevel != slog.LevelDebug {
		t.Errorf("main parsed wrong: %+v", cfg.Main)
	}

	if len(cfg.Main.ChatIDs) != 2 || cfg.Main.ChatIDs[1] != -100500 {
		t.Errorf("chat ids: %v", cfg.Main.ChatIDs)
	}

	if cfg.ModuleEnabled("failover") {
		t.Error("failover must be disabled")
	}

	if !cfg.ModuleEnabled("devices") {
		t.Error("unlisted module must default to enabled")
	}

	if got := cfg.Module("failover").List("tunnel"); len(got) != 2 || got[0] != "awg0" {
		t.Errorf("tunnel list: %v", got)
	}

	if cfg.Notification("logins").Int("success_window", 0) != 10 {
		t.Error("notify section not parsed")
	}

	if len(cfg.Commands) != 1 || cfg.Commands[0].Name != "speed" || !cfg.Commands[0].Confirm || cfg.Commands[0].Timeout.Seconds() != 90 {
		t.Errorf("commands: %+v", cfg.Commands)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Parallel()

	env := map[string]string{config.EnvChatIDs: "7, 8", config.EnvLogLevel: "error"}

	cfg, err := config.Load(context.Background(), staticSource{samplePackage()}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Main.ChatIDs) != 2 || cfg.Main.ChatIDs[0] != 7 || cfg.Main.ChatIDs[1] != 8 {
		t.Errorf("env chat ids not applied: %v", cfg.Main.ChatIDs)
	}

	if cfg.Main.LogLevel != slog.LevelError {
		t.Errorf("env log level not applied: %v", cfg.Main.LogLevel)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr bool
		wantWrn int
	}{
		{name: "valid", mutate: func(*config.Config) {}},
		{name: "no token", mutate: func(c *config.Config) { c.Main.Token = "" }, wantErr: true},
		{name: "bad token", mutate: func(c *config.Config) { c.Main.Token = "abc" }, wantErr: true},
		{name: "bad proxy", mutate: func(c *config.Config) { c.Main.Proxy = "ftp://x" }, wantErr: true},
		{name: "socks proxy ok", mutate: func(c *config.Config) { c.Main.Proxy = "socks5h://h:1" }},
		{name: "empty chat ids warns", mutate: func(c *config.Config) { c.Main.ChatIDs = nil }, wantWrn: 1},
		{name: "command clashes with builtin", mutate: func(c *config.Config) { c.Commands[0].Name = "status" }, wantErr: true},
		{name: "bad command name", mutate: func(c *config.Config) { c.Commands[0].Name = "Speed-Test" }, wantErr: true},
	}

	builtin := func(n string) bool { return n == "status" }

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.Parse(samplePackage())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			tt.mutate(&cfg)

			warns, err := cfg.Validate(builtin)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}

			if len(warns) != tt.wantWrn {
				t.Errorf("warnings = %v, want %d", warns, tt.wantWrn)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	pkg := samplePackage()
	pkg.Sections["main"].Lists["chat_id"] = []string{"notanumber"}

	_, err := config.Parse(pkg)
	if err == nil {
		t.Fatal("expected an error for a non-numeric chat id")
	}

	var target interface{ Error() string }
	if !errors.As(err, &target) {
		t.Fatal("error must be wrapped")
	}
}
