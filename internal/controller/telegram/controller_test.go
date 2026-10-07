package telegram_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	tgctl "github.com/emgeorrk/wrtgram/internal/controller/telegram"
	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase/mocks"
)

type clock struct{}

func (clock) Now() time.Time                         { return time.Now() }
func (clock) After(d time.Duration) <-chan time.Time { return time.After(d) }

type fakeModule struct{ cmds []module.Command }

func (fakeModule) Name() string                 { return "fake" }
func (fakeModule) Detect(context.Context) bool  { return true }
func (m fakeModule) Commands() []module.Command { return m.cmds }
func (fakeModule) Notifiers() []module.Notifier { return nil }

type source struct{ ch chan entity.Update }

func (s source) Updates(context.Context) <-chan entity.Update { return s.ch }

var errFail = errors.New("fail")

func newRegistry(t *testing.T, cmds ...module.Command) *module.Registry {
	t.Helper()

	reg := module.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := reg.Add(context.Background(), fakeModule{cmds: cmds}, true); err != nil {
		t.Fatal(err)
	}

	return reg
}

func echoCmd() module.Command {
	return module.Command{Name: "echo", Description: "echo args", Handle: func(_ context.Context, r module.Request) (module.Reply, error) {
		return module.Reply{Text: "echo:" + strings.Join(r.Args, ",")}, nil
	}}
}

func TestDispatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		update   entity.Update
		allowed  bool
		noList   bool
		wantText string // substring of the sent message; "" = nothing sent
	}{
		{name: "command with args", update: entity.Update{Text: "/echo a b", ChatID: 1}, allowed: true, wantText: "echo:a,b"},
		{name: "bot suffix and case", update: entity.Update{Text: "/Echo@mybot x", ChatID: 1}, allowed: true, wantText: "echo:x"},
		{name: "help lists commands", update: entity.Update{Text: "/help", ChatID: 1}, allowed: true, wantText: "/echo — echo args"},
		{name: "unknown command", update: entity.Update{Text: "/nope", ChatID: 1}, allowed: true, wantText: "Unknown command /nope"},
		{name: "plain text in private chat", update: entity.Update{Text: "hi", ChatID: 1, Private: true}, allowed: true, wantText: "see /help"},
		{name: "plain text in group ignored", update: entity.Update{Text: "hi", ChatID: 1}, allowed: true},
		{name: "unauthorized ignored", update: entity.Update{Text: "/echo", ChatID: 2}, allowed: false},
		{name: "no allowlist tells the chat id", update: entity.Update{Text: "/echo", ChatID: 77}, noList: true, wantText: "Your chat id is <code>77</code>"},
		{name: "handler error", update: entity.Update{Text: "/fail", ChatID: 1}, allowed: true, wantText: "⚠️ fail"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			sender := mocks.NewMockSender(ctrl)
			sender.EXPECT().ChatAction(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

			sent := make(chan string, 4)

			if tt.wantText != "" {
				sender.EXPECT().SendMessage(gomock.Any(), tt.update.ChatID, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ int64, m entity.Message) (int, error) {
						sent <- m.Text

						return 1, nil
					})
			}

			failCmd := module.Command{Name: "fail", Description: "fails", Handle: func(context.Context, module.Request) (module.Reply, error) {
				return module.Reply{}, errFail
			}}

			src := source{ch: make(chan entity.Update, 1)}
			c := tgctl.New(sender, src, newRegistry(t, echoCmd(), failCmd), clock{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
				tgctl.Options{Allowed: func(id int64) bool { return tt.allowed && id == 1 }, NoAllowlist: tt.noList, Workers: 2})

			src.ch <- tt.update
			close(src.ch)

			c.Run(context.Background())

			if tt.wantText == "" {
				return
			}

			select {
			case got := <-sent:
				if !strings.Contains(got, tt.wantText) {
					t.Errorf("sent %q, want substring %q", got, tt.wantText)
				}
			default:
				t.Fatal("nothing was sent")
			}
		})
	}
}

func TestConfirmFlow(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sender := mocks.NewMockSender(ctrl)
	sender.EXPECT().ChatAction(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	prompted := make(chan [][]entity.Button, 1)

	sender.EXPECT().SendMessage(gomock.Any(), int64(1), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int64, m entity.Message) (int, error) {
			prompted <- m.Keyboard

			return 10, nil
		})

	edited := make(chan string, 1)
	sender.EXPECT().EditMessage(gomock.Any(), int64(1), 10, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int64, _ int, m entity.Message) error {
			edited <- m.Text

			return nil
		})
	sender.EXPECT().AnswerCallback(gomock.Any(), "cb1", "").Return(nil)

	danger := module.Command{Name: "danger", Description: "d", Confirm: true, Handle: func(_ context.Context, r module.Request) (module.Reply, error) {
		if !r.FromCallback {
			return module.Reply{Text: "Really?"}, nil
		}

		return module.Reply{Text: "done"}, nil
	}}

	src := source{ch: make(chan entity.Update, 2)}
	c := tgctl.New(sender, src, newRegistry(t, danger), clock{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		tgctl.Options{Allowed: func(int64) bool { return true }, Workers: 1})

	src.ch <- entity.Update{Text: "/danger", ChatID: 1, MessageID: 5}

	go c.Run(context.Background())

	// Wait for the prompt with its keyboard, then press Yes.
	var keyboard [][]entity.Button

	select {
	case keyboard = <-prompted:
	case <-time.After(5 * time.Second):
		t.Fatal("no confirmation prompt")
	}

	if len(keyboard) != 1 || len(keyboard[0]) != 2 || !strings.HasPrefix(keyboard[0][0].Data, "danger:yes:") {
		t.Fatalf("keyboard = %+v", keyboard)
	}

	src.ch <- entity.Update{CallbackID: "cb1", CallbackData: keyboard[0][0].Data, ChatID: 1, MessageID: 10}
	close(src.ch)

	select {
	case got := <-edited:
		if got != "done" {
			t.Errorf("edited text = %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("confirmation not executed")
	}
}
