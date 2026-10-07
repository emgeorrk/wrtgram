# wrtgram

Telegram bot for OpenWrt routers: status, connected devices, VPN tunnels with
automatic failover, encrypted config backups and security notifications —
in a single static binary with no dependencies.

> Work in progress. The first release will ship prebuilt binaries for the
> common OpenWrt architectures (arm64, armv7, armv5, mipsle, mips, x86_64) and
> an `install.sh`.

## Requirements

- OpenWrt 23.05 or newer.
- About 10 MB of free space on the overlay and 20 MB of free RAM.
  Devices with 16 MB of flash are not supported.
