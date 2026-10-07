package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/emgeorrk/wrtgram/config"
	"github.com/emgeorrk/wrtgram/internal/controller/ipc"
	"github.com/emgeorrk/wrtgram/internal/repo/execx"
	"github.com/emgeorrk/wrtgram/internal/repo/ubus"
	"github.com/emgeorrk/wrtgram/internal/repo/uci"
	"github.com/emgeorrk/wrtgram/internal/usecase"
	"github.com/emgeorrk/wrtgram/pkg/logger"
)

// Environment variables of the host dev mode.
const (
	envFake     = "WRTGRAM_FAKE"     // "1": replay fixtures instead of running router tools
	envFixtures = "WRTGRAM_FIXTURES" // fixture file (default internal/repo/execx/testdata/router.txt)
	envRootFS   = "WRTGRAM_ROOTFS"   // directory standing in for / (default …/testdata/rootfs)
	envConfig   = "WRTGRAM_CONFIG"   // JSON file with a `uci get` reply instead of ubus
)

const (
	defaultFixtures = "internal/repo/execx/testdata/router.txt"
	defaultRootFS   = "internal/repo/execx/testdata/rootfs"
	overlayDir      = "/overlay"
)

// logOptions picks the daemon's log shape: stdout so procd forwards it at
// info priority, no timestamp because syslog adds one.
func logOptions(cfg config.Config, daemon bool) (io.Writer, logger.Options) {
	w := io.Writer(os.Stderr)
	if daemon {
		w = os.Stdout
	}

	return w, logger.Options{Level: cfg.Main.LogLevel, Secrets: []string{cfg.Main.Token}, OmitTime: daemon}
}

// env is everything built before the modules: adapters shared by run, probe
// and check-config.
type env struct {
	run     usecase.Runner
	fsys    fs.FS
	log     *slog.Logger
	ubus    *ubus.Client
	uci     *uci.Client
	overlay string
	cfg     config.Config
	fake    bool
}

// setup builds the shared adapters. With needConfig false a missing or broken
// /etc/config/wrtgram is reported and defaults are used (probe).
func setup(ctx context.Context, needConfig, daemon bool) (*env, error) {
	e := &env{fake: os.Getenv(envFake) == "1", overlay: overlayDir}

	if e.fake {
		if err := e.setupFake(); err != nil {
			return nil, err
		}
	} else {
		e.run = execx.New()
		e.fsys = os.DirFS("/")
	}

	if _, err := os.Stat(e.overlay); err != nil {
		e.overlay = "/"
	}

	e.ubus = ubus.New(e.run)
	e.uci = uci.New(e.ubus)

	var src config.Source = e.uci
	if p := os.Getenv(envConfig); p != "" {
		src = uci.File(p)
	}

	cfg, err := config.Load(ctx, src, os.Getenv)
	if err != nil {
		if needConfig {
			return nil, fmt.Errorf("%w: %w", errConfig, err)
		}

		cfg = config.Defaults()
	}

	e.log = logger.New(logOptions(cfg, daemon))

	if err != nil {
		e.log.Warn("config not loaded, using defaults", "err", err)
	}

	e.cfg = cfg

	if e.fake {
		e.log.Info("fake mode: replaying fixtures", "fixtures", envOr(envFixtures, defaultFixtures))
	}

	return e, nil
}

func (e *env) setupFake() error {
	path := envOr(envFixtures, defaultFixtures)

	fake, err := execx.LoadFixtures(path)
	if err != nil {
		return fmt.Errorf("%w: %w", errNoFixtures, err)
	}

	root, err := filepath.Abs(envOr(envRootFS, defaultRootFS))
	if err != nil {
		return fmt.Errorf("%w: %w", errNoFixtures, err)
	}

	e.run = fake
	e.fsys = os.DirFS(root)

	return nil
}

// tmpDir is where fake mode keeps volatile state (the host's temp dir).
func (e *env) tmpDir() string { return os.TempDir() }

// socket is the IPC socket path (a temp path in fake mode).
func (e *env) socket() string {
	if e.fake {
		return filepath.Join(e.tmpDir(), "wrtgram.sock")
	}

	return ipc.DefaultSocket
}

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}

	return def
}
