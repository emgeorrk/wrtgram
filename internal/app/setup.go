package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/config"
	"github.com/emgeorrk/wrtgram/internal/entity"
	tgrepo "github.com/emgeorrk/wrtgram/internal/repo/telegram"
)

const (
	minPassword = 8
	chatWait    = 20 * time.Second
)

var (
	errSetupFake    = errors.New("setup needs a real router (not WRTGRAM_FAKE)")
	errSetupAbort   = errors.New("setup aborted")
	errNoChat       = errors.New("no message from you was found")
	errShortPass    = errors.New("password is shorter than 8 characters")
	errPassMismatch = errors.New("passwords do not match")
)

// wizard is the interactive `wrtgram setup`.
type wizard struct {
	e   *env
	in  *bufio.Reader
	out io.Writer
}

// setupCmd asks for the token and the backup password, finds the owner's
// chat id and writes everything to UCI. Secrets are typed with echo off and
// travel to UCI through ubus, never through a command line.
func setupCmd(ctx context.Context) error {
	if os.Getenv(envFake) == "1" {
		return errSetupFake
	}

	e, err := setup(ctx, false, false)
	if err != nil {
		return err
	}

	w := &wizard{e: e, in: bufio.NewReader(os.Stdin), out: os.Stdout}

	fmt.Fprintln(w.out, "=== wrtgram setup ===")

	token, tg, botName, err := w.askToken(ctx)
	if err != nil {
		return err
	}

	chat, err := w.askChat(ctx, tg, botName)
	if err != nil {
		return err
	}

	password, err := w.askPassword()
	if err != nil {
		return err
	}

	if err := w.save(ctx, token, chat, password); err != nil {
		return err
	}

	fmt.Fprintln(w.out, "Restarting the service…")

	if _, err := e.run.Run(ctx, "/etc/init.d/wrtgram", "restart"); err != nil {
		return fmt.Errorf("restart: %w", err)
	}

	_, err = tg.SendMessage(ctx, chat, entity.Message{Text: "🤖 wrtgram is connected to this router. Send /help to see the commands."})
	if err != nil {
		fmt.Fprintln(w.out, "warning: greeting not sent:", err)
	}

	fmt.Fprintln(w.out, "Done. Open the bot in Telegram and send /status.")

	return nil
}

func (w *wizard) askToken(ctx context.Context) (token string, tg *tgrepo.Client, botName string, err error) {
	if w.e.cfg.Main.Token != "" {
		fmt.Fprint(w.out, "A token is already configured. Replace it? [y/N] ")

		if !w.yes() {
			return "", nil, "", errSetupAbort
		}
	}

	for {
		fmt.Fprintln(w.out, "1. Create a bot with @BotFather (/newbot) and paste its token.")

		token, err = w.secret("   Token (input hidden): ")
		if err != nil {
			return "", nil, "", err
		}

		if !config.ValidToken(token) {
			fmt.Fprintln(w.out, "   That does not look like a token (expected 123456:ABC…). Try again.")

			continue
		}

		tg, err = tgrepo.New(token, w.e.cfg.Main.Proxy, w.e.log)
		if err != nil {
			return "", nil, "", err
		}

		me, err := tg.Me(ctx)
		if err != nil {
			fmt.Fprintln(w.out, "   Telegram rejected the token:", err)

			continue
		}

		fmt.Fprintf(w.out, "   ✓ token accepted: @%s\n", me.Username)

		return token, tg, me.Username, nil
	}
}

func (w *wizard) askChat(ctx context.Context, tg *tgrepo.Client, botName string) (int64, error) {
	fmt.Fprintf(w.out, "2. Open https://t.me/%s in Telegram, press Start (or send any message),\n   then press Enter here. ", botName)
	w.line()

	chat, user, err := tg.LastChat(ctx, chatWait)
	if err != nil {
		return 0, err
	}

	if chat != 0 {
		fmt.Fprintf(w.out, "   ✓ your chat id: %d (@%s)\n", chat, user)

		return chat, nil
	}

	fmt.Fprint(w.out, "   No message found. Type your chat id manually (or Enter to abort): ")

	raw := w.line()
	if raw == "" {
		return 0, errNoChat
	}

	manual, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", errNoChat, raw)
	}

	return manual, nil
}

func (w *wizard) askPassword() (string, error) {
	fmt.Fprintln(w.out, "3. Password for encrypting configuration backups (8+ characters; Enter = unencrypted).")

	p1, err := w.secret("   Password: ")
	if err != nil {
		return "", err
	}

	if p1 == "" {
		return "", nil
	}

	if len(p1) < minPassword {
		return "", errShortPass
	}

	p2, err := w.secret("   Repeat:   ")
	if err != nil {
		return "", err
	}

	if p1 != p2 {
		return "", errPassMismatch
	}

	return p1, nil
}

func (w *wizard) save(ctx context.Context, token string, chat int64, password string) error {
	uci := w.e.uci

	// Creating a section that exists fails; that is fine, the set follows.
	w.ensure(ctx, config.TypeModule, "backup")
	w.ensure(ctx, config.TypeMain, config.SectionMain)

	main := map[string]any{"enabled": "1", "token": token, "chat_id": []string{strconv.FormatInt(chat, 10)}}
	if err := uci.Set(ctx, config.Package, config.SectionMain, main); err != nil {
		return err
	}

	if err := uci.Set(ctx, config.Package, "backup", map[string]any{"password": password}); err != nil {
		return err
	}

	if err := uci.Commit(ctx, config.Package); err != nil {
		return err
	}

	fmt.Fprintln(w.out, "4. Saved to /etc/config/wrtgram.")

	return nil
}

func (w *wizard) ensure(ctx context.Context, typ, name string) {
	if err := w.e.uci.AddSection(ctx, config.Package, typ, name); err != nil {
		w.e.log.Debug("uci add", "section", name, "err", err)
	}
}

// secret reads a line with terminal echo switched off.
func (w *wizard) secret(prompt string) (string, error) {
	fmt.Fprint(w.out, prompt)

	w.echo(false)

	line, err := w.in.ReadString('\n')

	w.echo(true)

	fmt.Fprintln(w.out)

	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read input: %w", err)
	}

	return strings.TrimSpace(line), nil
}

// echo toggles terminal echo; without a tty stty fails and input stays visible.
func (w *wizard) echo(on bool) {
	arg := "-echo"
	if on {
		arg = "echo"
	}

	if _, err := w.e.run.RunInput(context.Background(), os.Stdin, "stty", arg); err != nil {
		w.e.log.Debug("stty", "err", err)
	}
}

func (w *wizard) line() string {
	line, err := w.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}

	return strings.TrimSpace(line)
}

func (w *wizard) yes() bool {
	switch strings.ToLower(w.line()) {
	case "y", "yes":
		return true
	}

	return false
}
