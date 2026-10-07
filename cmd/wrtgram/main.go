// Command wrtgram is a Telegram bot for OpenWrt routers.
//
// Usage:
//
//	wrtgram [run]                 start the daemon (procd)
//	wrtgram notify <text>         send a notification through the running daemon
//	wrtgram send-file <path> [caption]
//	wrtgram event dhcp            report a dnsmasq hotplug event (called from /etc/hotplug.d)
//	wrtgram probe                 print the detected modules and sample data
//	wrtgram check-config          validate /etc/config/wrtgram
//	wrtgram setup                 interactive setup (token, chat id, backup password)
//	wrtgram version
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/emgeorrk/wrtgram/internal/app"
)

// version is stamped by the linker: -X main.version=v1.2.3.
var version = "dev"

func main() {
	args := os.Args[1:]

	// Installed as /usr/bin/tg-notify (symlink) it behaves like `wrtgram notify`
	// so scripts written for the shell bot keep working.
	if filepath.Base(os.Args[0]) == "tg-notify" {
		args = append([]string{"notify"}, args...)
	}

	cmd := "run"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	if err := app.Main(cmd, args, version); err != nil {
		fmt.Fprintln(os.Stderr, "wrtgram:", err)
		os.Exit(1)
	}
}
