# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

**wrtgram** — a Telegram bot for OpenWrt routers, written in Go, shipped as a
single static binary for every common OpenWrt architecture. It replaces a set
of router-specific shell scripts with a modular daemon: each feature module
detects whether the router has the tools it needs and only then registers its
commands and notifiers. English UI only.

Target devices run busybox userland as root. Everything is reached through
`ubus call`, `uci` (via ubus), `/proc`, `/sys`, `logread -f`, `ip`, `ping`,
`wg`/`awg`, `sysupgrade`. No cgo, no shell for argv (except user-defined
custom commands, which are shell by design), no polling faster than needed.

## Build / test / run

```sh
make build          # host binary → bin/wrtgram
make build-router   # static linux/arm64 (ROUTER_GOARCH=mipsle etc.)
make dist           # every target into dist/
make size           # router binary size against the budget (SIZE_MAX)
make test           # go test -race ./...
make lint           # golangci-lint run (lint-fix = with --fix)
make generate       # regenerate gomock mocks (go generate ./...)
make deploy ROUTER=192.168.1.1   # scp to /tmp/wrtgram, run probe + check-config
make install-remote              # install into /usr/bin and restart the service
make rss                         # VmRSS of the running bot on the router
make run-fake       # run on the host against recorded fixtures (WRTGRAM_FAKE=1)
```

`go.mod` says `go 1.23` on purpose: the OpenWrt 24.10 SDK ships Go 1.23.12 and
must be able to build the package. No `tool` directive, no stdlib
`crypto/pbkdf2`, no `testing/synctest`. Bump when 24.10 is EOL.

## Architecture

Clean-architecture layers (evrone/go-clean-template), one-directional:

```
cmd/wrtgram            main: subcommand switch (run | notify | send-file | event | probe | check-config | setup | version)
  → internal/app       wiring: config → adapters → modules.Detect → registry → telegram → notifiers → IPC
    → internal/controller/telegram  update dispatcher: auth → parse → per-chat workers → dispatch → render → send
    → internal/controller/ipc       unix socket for the CLI (notify, send-file, event dhcp)
    → internal/module/<name>        feature modules: Detect + Commands + Notifiers + rendering (the ONLY layer that knows Telegram HTML)
      → internal/usecase/<name>     rules (failover state machine, device merge, throttling, hysteresis) — depend only on entity + ports
        → internal/repo/<name>      adapters: execx, ubus, uci, sysfs, netifd, wg, iproute, wifi, dhcp, logread, sysupgrade, state, telegram
internal/entity        domain types, zero deps
internal/usecase/contracts.go   every port; mocks generated into internal/usecase/mocks
config                 UCI package `wrtgram` → typed Config (+ env overrides for dev)
pkg/{logger,throttle,tgtext,osslenc}   generic helpers
package/wrtgram        OpenWrt package: Makefile + procd init, config, uci-defaults, hotplug
scripts/               install.sh, deploy.sh, fixtures.sh
```

**Capability gating is the core pattern.** `module.Module.Detect(ctx)` checks
for the binary / ubus object / init script. The registry skips modules that
are disabled in UCI or not detected — logged, never an error. A missing tool
hides its module; it must never crash the bot.

**Adding a module:** package `internal/module/<name>` implementing
`module.Module`; rules in `internal/usecase/<name>`; new external data goes
through a port in `usecase/contracts.go` with an adapter in `internal/repo`;
register the module in `internal/app`; add a UCI `config module '<name>'`
example to `package/wrtgram/files/wrtgram.config` and the README.

## Conventions (enforced by .golangci.yml)

- **Errors — sentinels only.** Each package declares `var errFoo = errors.New(…)`
  in `errors.go`; wrap with `fmt.Errorf("…: %w", errFoo)`. Test files are exempt.
- **Tests — table-driven, `t.Parallel()`** at both levels. Mocks come from
  `go generate` (mockgen from `usecase/contracts.go`); never hand-write fakes
  for ports, except `execx.Fake`, which is the fixture player for dev mode.
- **Rendering**: every dynamic string goes through `tgtext.Esc`; only
  `<b> <i> <code> <pre>` are emitted; long texts via `tgtext.Split`.
- **Secrets**: the token never appears in argv, logs (logger redacts it) or
  error strings; `/etc/config/wrtgram` is mode 600.
- **Size**: zero production deps besides go-telegram/bot; no `text/template`
  or `regexp` in production code. `make size` must pass.
- Linting is strict (`gofumpt`, `fieldalignment`, `wsl_v5`, `funlen` 65/40,
  `gocyclo` 10, `mnd`, `err113`, `goconst`). Deliberate exceptions carry
  `//nolint:<linter> // reason`. Run `make lint` before finishing.

## Runtime facts

- Daemon paths: `/etc/config/wrtgram` (UCI, 600), `/etc/wrtgram/` (persistent
  state: `manual.json`, `backup.json`, `known_macs`), `/var/run/wrtgram/`
  (volatile: `failover.json`, temporary backups), `/var/run/wrtgram.sock` (IPC).
- `uci commit` through rpcd fires a `config.change` event and procd restarts
  the service (reload trigger). That is why the manual VPN switch is a state
  file, not a UCI option.
- Notifiers run under a supervisor with backoff; the queue retries sends,
  honours 429, and lets an in-flight send finish during shutdown.

## Router facts worth remembering

- hostapd ubus objects are `hostapd.wlan0` on 23.05 and `hostapd.phy0-ap0` on
  24.10+: always enumerate with `ubus list 'hostapd.*'`.
- `awg show <if> dump` prints ~30 fields on the interface line (wg prints 5);
  peer lines are identical (9 fields).
- Routers have no RTC: TLS to Telegram fails until NTP syncs. Retry `getMe`
  with backoff; never schedule by wall clock before the year looks sane.
- busybox: no `timeout`, `pkill`, `ip -j`; `logread -f` replays the buffer.
