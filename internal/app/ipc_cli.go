package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/controller/ipc"
	"github.com/emgeorrk/wrtgram/internal/entity"
	tgrepo "github.com/emgeorrk/wrtgram/internal/repo/telegram"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

// notifyCmd sends a text through the daemon; without a daemon it talks to
// Telegram directly (scripts keep working while the bot is stopped).
func notifyCmd(ctx context.Context, args []string) error {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		return fmt.Errorf("%w: wrtgram notify <text>", errUsage)
	}

	err := ipc.Call(ctx, socketPath(), ipc.Request{Op: ipc.OpNotify, Text: text})
	if !errors.Is(err, ipc.ErrNoDaemon) {
		return err
	}

	return direct(ctx, func(ctx context.Context, tg *tgrepo.Client, chat int64) error {
		_, err := tg.SendMessage(ctx, chat, entity.Message{Text: tgtext.Esc(text)})

		return err
	})
}

// sendFileCmd sends a file the same way.
func sendFileCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: wrtgram send-file <path> [caption]", errUsage)
	}

	path, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("send-file: %w", err)
	}

	caption := strings.Join(args[1:], " ")

	err = ipc.Call(ctx, socketPath(), ipc.Request{Op: ipc.OpFile, Path: path, Caption: caption})
	if !errors.Is(err, ipc.ErrNoDaemon) {
		return err
	}

	return direct(ctx, func(ctx context.Context, tg *tgrepo.Client, chat int64) error {
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("send-file: %w", err)
		}
		defer f.Close()

		return tg.SendDocument(ctx, chat, entity.Document{Data: f, Name: filepath.Base(path), Caption: caption})
	})
}

// eventCmd forwards a hotplug event (environment of /etc/hotplug.d/dhcp).
// Without a daemon it exits quietly: nobody is listening.
func eventCmd(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "dhcp" {
		return fmt.Errorf("%w: wrtgram event dhcp", errUsage)
	}

	ev := entity.DHCPEvent{
		Action:   os.Getenv("ACTION"),
		MAC:      os.Getenv("MACADDR"),
		IP:       os.Getenv("IPADDR"),
		Hostname: os.Getenv("HOSTNAME"),
	}

	if ev.MAC == "" {
		return nil
	}

	err := ipc.Call(ctx, socketPath(), ipc.Request{Op: ipc.OpEvent, Event: &ev})
	if errors.Is(err, ipc.ErrNoDaemon) {
		return nil
	}

	return err
}

// direct loads the config and sends to every notification chat itself.
func direct(ctx context.Context, send func(ctx context.Context, tg *tgrepo.Client, chat int64) error) error {
	e, err := setup(ctx, true, false)
	if err != nil {
		return err
	}

	tg, err := tgrepo.New(e.cfg.Main.Token, e.cfg.Main.Proxy, e.log)
	if err != nil {
		return err
	}

	for _, chat := range e.notifyChats() {
		if err := send(ctx, tg, chat); err != nil {
			return err
		}
	}

	return nil
}

func socketPath() string {
	if os.Getenv(envFake) == "1" {
		return filepath.Join(os.TempDir(), "wrtgram.sock")
	}

	return ipc.DefaultSocket
}
