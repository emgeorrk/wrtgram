# wrtgram

Telegram bot for OpenWrt routers: status, connected devices, VPN tunnels with
automatic failover, encrypted configuration backups and security
notifications — one static binary, no dependencies, modules that enable
themselves based on what the router has installed.

```
/status      📡 Cudy WR3000S v1
             OpenWrt · OpenWrt 25.12.5 r33051-f5dae5ece4
             Uptime: 16h 21m, load 0.02 0.02 0.00
             Memory: 36.7 MiB free of 234.0 MiB
             Flash: 35.2 MiB free of 44.2 MiB
             🌡 CPU 66°C, Wi-Fi 0 60°C, Wi-Fi 1 60°C
             WAN: ✅ up 16h 1m · 10.0.0.2

             🛡 VPN: primary awg0 ✅ (for 3h 12m)
```

## Features

| Command | What it does | Needs |
|---|---|---|
| `/status` | model, release, uptime, load, memory, flash, temperatures, WAN, VPN | — |
| `/wan` | upstream address, gateway, DNS | — |
| `/devices` | DHCP leases merged with Wi-Fi associations, paginated; 📌 static lease, ⛔ blocked | dnsmasq and/or hostapd |
| `/blocked` | blocked devices with Unblock buttons | — |
| `/vpn`, `/vpn_off`, `/vpn_on` | tunnel state, switch the VPN off and on | `wg` or `awg` |
| `/failover` | failover details: roles, timers, routes | failover configured |
| `/backup` | `sysupgrade -b` archive, AES-256 encrypted, sent as a file | `sysupgrade` |
| `/temp` | every hwmon sensor | hwmon sensors |
| `/services`, `/restart <name>` | procd services, restart whitelisted ones | — |
| `/reboot` | reboot with a confirmation button | — |
| `/<name>` | your own shell commands from UCI | `config command` |
| `/help` | the list above, built from the active modules | — |

Notifications: router started, VPN failover switches, new device on the LAN
(with **📌 Remember IP** — a static DHCP lease in `/etc/config/dhcp` — and
**⛔ Block** — fw4 rules rejecting the MAC towards every zone and the router),
SSH and LuCI logins (and failed attempts, rate-limited per address),
overheating with an all-clear, and a weekly encrypted backup.

A built-in **VPN failover** keeps the default route on the best live tunnel:
primary first, a backup after the primary has been down for a grace period,
straight back to the primary when it recovers, and the bot's own traffic
always through a live tunnel so it keeps working where Telegram is blocked.

## Requirements

- OpenWrt 23.05 or newer with `ubus` (any image has it).
- About 10 MB of free overlay space and 20 MB of free RAM (the binary is
  7 MB, ~4 MB on a compressed overlay; RSS is about 12 MB). Devices with
  16 MB of flash are not supported.
- For the failover: `ip-full` (`opkg/apk install ip-full`) when `ntp_direct`
  is on; every tunnel's peers must have `option route_allowed_ips '0'`,
  because wrtgram owns the default routes.

## Install

On the router, as root:

```sh
sh -c "$(wget -qO- https://raw.githubusercontent.com/emgeorrk/wrtgram/main/scripts/install.sh)"
```

The installer detects the architecture, downloads the matching binary from
the latest release, verifies it against `checksums.txt`, installs the procd
service and runs `wrtgram setup`:

1. paste the token from [@BotFather](https://t.me/BotFather) (typed with echo off);
2. press Start in the bot — the wizard finds your chat id;
3. choose a password for encrypted backups.

Other ways:

- `sh install.sh --version v0.1.0` pins a release; `--url` installs a tarball from elsewhere; `--no-wizard` skips the setup.
- Packages built with the OpenWrt SDK (`.ipk` for 24.10, `.apk` for 25.x) are attached to releases for the most common architectures; unsigned `.apk` files need `apk add --allow-untrusted`.
- From source: `GOOS=linux GOARCH=arm64 make build-router` (see the Makefile for `GOARM`/`GOMIPS` of other targets), copy `bin/wrtgram-linux-arm64` to `/usr/bin/wrtgram` and `package/wrtgram/files/*` to their places.
- Remove: `sh install.sh --uninstall` (keeps `/etc/config/wrtgram`).

## Configuration

Everything lives in `/etc/config/wrtgram` (mode 600). After editing run
`/etc/init.d/wrtgram restart`; `wrtgram check-config` validates the file and
`wrtgram probe` shows which modules the router supports.

```
config main 'main'
	option token ''             # from @BotFather
	list chat_id '123456789'    # chats allowed to use the bot; empty = the bot only tells you your id
	list notify_chat_id ''      # where notifications go (default: chat_id)
	option proxy ''             # http://user:pass@host:port or socks5h://host:1080
	option log_level 'info'
	option workers '2'
	option wan_interface ''     # empty = detect
```

Modules are `config module '<name>'` sections, all enabled by default and
skipped automatically when the router lacks the tool they need:

| Section | Options |
|---|---|
| `devices` | `page_size` (25) |
| `failover` | `enabled` (0), `list tunnel` in priority order, `list target` ping targets, `grace` (300 s), `interval` (30 s), `ntp_direct` (1), `list service_net` (Telegram ranges) |
| `backup` | `password` — empty means unencrypted archives |
| `services` | `list service` — names `/restart` may restart |
| `system`, `network`, `vpn`, `thermal`, `logins`, `custom` | `enabled` only |

Custom commands become bot commands:

```
config command 'speed'
	option command '/usr/bin/speedtest-cli --simple'
	option description 'Run a speed test'
	option timeout '90'
	option confirm '0'          # 1 = ask before running
```

Notification rules are `config notify '<name>'` sections:

| Section | Options |
|---|---|
| `boot` | `enabled` |
| `new_device` | `enabled`, `known_file` (`/etc/wrtgram/known_macs`; empty the file and restart to get a card for every device again) |
| `logins` | `enabled`, `success_window` (3600 s per IP), `failure_window` (600 s), `list trusted_ip` |
| `thermal` | `enabled`, `high` (85), `normal` (75), `interval` (60 s), `remind` (3600 s) |
| `backup` | `enabled`, `day` (`sun`), `time` (`03:30`) |
| `failover` | `enabled` |

### VPN failover

```
config module 'failover'
	option enabled '1'
	list tunnel 'wg0'       # primary
	list tunnel 'wg1'       # backup
	option grace '300'
```

Every `interval` seconds each tunnel is pinged through its interface. The
primary wins whenever it answers. If it stops answering, traffic stays on it
for `grace` seconds (no flapping), then moves to the first healthy backup, or
goes direct when nothing answers. Networks in `service_net` (Telegram by
default) always use a live tunnel, even during the grace and when the VPN is
switched off with `/vpn_off`, so the bot never loses contact.

`ntp_direct` sends the router's own NTP packets straight to the WAN gateway
(`ip rule` 29990 → table 123). Routers have no clock battery; after a power
cut the time is wrong, WireGuard peers reject the handshake, and NTP through
a dead tunnel would never fix it.

State is written to `/var/run/wrtgram/failover.json` for other scripts; the
manual switch lives in `/etc/wrtgram/manual.json`.

### Backups

`/backup` and the weekly job run `sysupgrade -b`, encrypt the archive in the
`openssl enc` format and send it to the chat. Decrypt anywhere with:

```sh
openssl enc -d -aes-256-cbc -pbkdf2 -iter 200000 -in backup-router-20261007-0330.tar.gz.enc -out backup.tar.gz
```

### Scripts and other tools

```sh
wrtgram notify "Rebooting the modem"        # text notification through the running bot
wrtgram send-file /tmp/report.txt "caption" # a file
wrtgram event dhcp                          # what /etc/hotplug.d/dhcp/90-wrtgram calls
```

`notify` and `send-file` talk to the daemon over `/var/run/wrtgram.sock` and
fall back to the Telegram API directly when the daemon is not running.

### Telegram blocked or filtered?

Set `option proxy 'socks5h://host:1080'` (or `http://`) in `main`, or route
Telegram's networks through a tunnel: the failover module does that for you
with its default `service_net` list.

## Security

- The token never appears in a command line, in logs (it is masked) or in
  error messages; the config file is mode 600.
- Only the chats in `chat_id` are served; everything else is ignored and
  logged once per 10 minutes. With an empty list the bot answers every chat
  with its id and does nothing else.
- Destructive commands (`/reboot`, `/restart`, confirmable custom commands)
  need a button press; the confirmation expires after 10 minutes.
- Backups contain Wi-Fi passwords and VPN keys: set a backup password.

## Development

```sh
make build            # host binary
make test lint        # go test -race, golangci-lint
make build-router     # linux/arm64 (ROUTER_GOARCH=mipsle …)
make size             # size budget check
make deploy ROUTER=192.168.1.1   # scp to /tmp and run probe + check-config there
WRTGRAM_FAKE=1 go run ./cmd/wrtgram probe   # run on a workstation against recorded fixtures
```

Fake mode replays `internal/repo/execx/testdata/router.txt` (command
outputs) and `testdata/rootfs/` (`/proc`, `/sys`, `/tmp`) instead of the
real router; `scripts/fixtures.sh` records a new set from a router. With
`WRTGRAM_TOKEN` and `WRTGRAM_CHAT_IDS` set, `make run-fake` runs the whole
bot on the workstation.

Layout: `cmd/wrtgram` → `internal/app` (wiring) → `internal/controller`
(Telegram dispatcher, IPC socket) → `internal/module/<name>` (commands,
notifiers, rendering) → `internal/usecase` (rules, ports) →
`internal/repo` (ubus, uci, wg, ip, logread, sysfs, telegram adapters) →
`internal/entity`. See `CLAUDE.md` for the conventions.

## License

MIT
