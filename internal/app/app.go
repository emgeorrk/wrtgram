// Package app wires the configuration, adapters, modules and the Telegram
// controller together and runs the chosen subcommand.
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/emgeorrk/wrtgram/internal/module"
)

// Main dispatches a subcommand.
func Main(cmd string, args []string, version string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "run":
		return runDaemon(ctx, version)
	case "probe":
		return probe(ctx)
	case "check-config":
		return checkConfig(ctx)
	case "setup":
		return setupCmd(ctx)
	case "notify":
		return notifyCmd(ctx, args)
	case "send-file":
		return sendFileCmd(ctx, args)
	case "event":
		return eventCmd(ctx, args)
	case "version", "-v", "--version":
		fmt.Fprintf(os.Stdout, "wrtgram %s (%s %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)

		return nil
	case module.HelpCommand, "-h", "--help":
		fmt.Fprint(os.Stdout, usage)

		return nil
	}

	return fmt.Errorf("%w: %s (see wrtgram help)", errUnknownCommand, cmd)
}

const usage = `Usage:
  wrtgram [run]                      start the daemon
  wrtgram notify <text>              send a notification via the running daemon
  wrtgram send-file <path> [caption] send a file via the running daemon
  wrtgram event dhcp                 report a dnsmasq hotplug event
  wrtgram probe                      print detected modules and sample data
  wrtgram check-config               validate /etc/config/wrtgram
  wrtgram setup                      interactive setup: token, chat id, backup password
  wrtgram version
`
