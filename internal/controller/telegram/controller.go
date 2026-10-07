// Package telegram is the inbound controller: it authorizes updates, parses
// commands and callbacks, dispatches them to module handlers on per-chat
// workers and renders the replies.
package telegram

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	"github.com/emgeorrk/wrtgram/internal/usecase"
	"github.com/emgeorrk/wrtgram/pkg/tgtext"
	"github.com/emgeorrk/wrtgram/pkg/throttle"
)

const (
	workerQueue     = 32
	handlerTimeout  = 5 * time.Minute
	typingAfter     = 2 * time.Second
	unauthWindow    = 10 * time.Minute
	chatIDHintEvery = time.Minute
	maxArgs         = 16
	// maxUpdateAge drops commands that waited in Telegram's queue while the
	// router was off, so a stale /reboot never fires after power-up.
	maxUpdateAge = 2 * time.Minute
)

// Options configure the controller.
type Options struct {
	Allowed func(chatID int64) bool
	// NoAllowlist makes the bot answer every chat with its id instead of
	// serving commands — the first-run experience.
	NoAllowlist bool
	Workers     int
}

// Controller implements the update loop.
type Controller struct {
	sender   usecase.Sender
	source   usecase.UpdateSource
	registry *module.Registry
	clock    usecase.Clock
	log      *slog.Logger
	limiter  *throttle.Limiter
	confirms *confirmStore
	opts     Options
}

// New creates a controller.
func New(sender usecase.Sender, source usecase.UpdateSource, registry *module.Registry,
	clock usecase.Clock, log *slog.Logger, opts Options,
) *Controller {
	if opts.Workers < 1 {
		opts.Workers = 1
	}

	return &Controller{
		sender:   sender,
		source:   source,
		registry: registry,
		clock:    clock,
		log:      log,
		limiter:  throttle.New(),
		confirms: newConfirmStore(),
		opts:     opts,
	}
}

// Run consumes updates until ctx is done. Updates of one chat are handled in
// order on the same worker; different chats run concurrently.
func (c *Controller) Run(ctx context.Context) {
	queues := make([]chan entity.Update, c.opts.Workers)

	var wg sync.WaitGroup

	for i := range queues {
		queues[i] = make(chan entity.Update, workerQueue)

		wg.Add(1)

		go func(q <-chan entity.Update) {
			defer wg.Done()

			for u := range q {
				c.handle(ctx, u)
			}
		}(queues[i])
	}

	for u := range c.source.Updates(ctx) {
		q := queues[chatHash(u.ChatID)%uint32(len(queues))] //nolint:gosec // len fits in uint32

		select {
		case q <- u:
		default:
			c.log.Warn("worker queue full, update dropped", "chat", u.ChatID)
		}
	}

	for _, q := range queues {
		close(q)
	}

	wg.Wait()
}

// MenuCommands lists what goes into setMyCommands and /help.
func (c *Controller) MenuCommands() []entity.BotCommand {
	var out []entity.BotCommand

	for _, cmd := range c.registry.Commands() {
		if !cmd.Hidden {
			out = append(out, entity.BotCommand{Name: cmd.Name, Description: cmd.Description})
		}
	}

	return append(out, entity.BotCommand{Name: module.HelpCommand, Description: "List commands"})
}

// SetMenu publishes the command menu.
func (c *Controller) SetMenu(ctx context.Context) error {
	return c.sender.SetCommands(ctx, c.MenuCommands())
}

func (c *Controller) handle(ctx context.Context, u entity.Update) {
	ctx, cancel := context.WithTimeout(ctx, handlerTimeout)
	defer cancel()

	if !c.authorized(ctx, u) {
		return
	}

	if !u.IsCallback() && !u.At.IsZero() && c.clock.Now().Sub(u.At) > maxUpdateAge {
		c.log.Info("ignoring stale update", "chat", u.ChatID, "age", c.clock.Now().Sub(u.At).Round(time.Second))

		return
	}

	if u.IsCallback() {
		c.handleCallback(ctx, u)

		return
	}

	name, args, ok := parseCommand(u.Text)
	if !ok {
		if u.Private {
			c.reply(ctx, u, module.Reply{Text: "Send a command, see /help."})
		}

		return
	}

	c.handleCommand(ctx, u, name, args)
}

func (c *Controller) handleCommand(ctx context.Context, u entity.Update, name string, args []string) {
	switch name {
	case module.HelpCommand, module.StartCommand:
		c.reply(ctx, u, module.Reply{Text: c.helpText()})

		return
	}

	cmd, found := c.registry.Lookup(name)
	if !found {
		c.reply(ctx, u, module.Reply{Text: fmt.Sprintf("Unknown command /%s, see /help.", tgtext.Esc(name))})

		return
	}

	req := module.Request{Args: args, ChatID: u.ChatID, MessageID: u.MessageID}

	reply := c.invoke(ctx, cmd.Handle, req, u.ChatID)
	if cmd.Confirm && reply.Document == nil {
		nonce := c.confirms.add(cmd.Name, u.ChatID, args, c.clock.Now())
		reply.Keyboard = [][]entity.Button{{
			{Text: "✅ Yes", Data: cmd.Name + ":" + confirmYes + ":" + nonce},
			{Text: "✖️ Cancel", Data: cmd.Name + ":" + confirmNo + ":" + nonce},
		}}
	}

	c.reply(ctx, u, reply)
}

func (c *Controller) handleCallback(ctx context.Context, u entity.Update) {
	name, payload, _ := strings.Cut(u.CallbackData, ":")

	cmd, found := c.registry.Lookup(name)
	if !found {
		c.answer(ctx, u.CallbackID, "This button has expired.")

		return
	}

	req := module.Request{Payload: payload, ChatID: u.ChatID, MessageID: u.MessageID, FromCallback: true}

	var reply module.Reply

	switch {
	case cmd.Confirm:
		action, nonce, _ := strings.Cut(payload, ":")

		args, ok := c.confirms.take(nonce, cmd.Name, u.ChatID, c.clock.Now())
		if !ok {
			c.answer(ctx, u.CallbackID, "This confirmation has expired, send the command again.")

			return
		}

		req.Args = args

		if action != confirmYes {
			reply = module.Reply{Text: "Canceled.", Edit: true}

			break
		}

		reply = c.invoke(ctx, cmd.Handle, req, u.ChatID)
		reply.Edit = true
	case cmd.Callback != nil:
		reply = c.invoke(ctx, cmd.Callback, req, u.ChatID)
	default:
		c.answer(ctx, u.CallbackID, "")

		return
	}

	c.answer(ctx, u.CallbackID, reply.Toast)
	c.reply(ctx, u, reply)
}

// invoke runs a handler with a typing indicator and panic protection.
func (c *Controller) invoke(ctx context.Context, h module.HandlerFunc, req module.Request, chatID int64) (reply module.Reply) {
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-time.After(typingAfter):
			c.action(ctx, chatID, "typing")
		case <-done:
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			c.log.Error("handler panicked", "panic", r)

			reply = module.Reply{Text: "⚠️ " + tgtext.Esc(errHandlerPanic.Error())}
		}
	}()

	reply, err := h(ctx, req)
	if err != nil {
		c.log.Error("command failed", "err", err, "chat", chatID)

		return module.Reply{Text: "⚠️ " + tgtext.Esc(err.Error())}
	}

	return reply
}

func (c *Controller) reply(ctx context.Context, u entity.Update, r module.Reply) {
	if r.Document != nil {
		c.action(ctx, u.ChatID, "upload_document")

		if err := c.sender.SendDocument(ctx, u.ChatID, *r.Document); err != nil {
			c.log.Error("send document", "err", err, "chat", u.ChatID)
		}

		return
	}

	if r.Text == "" {
		return
	}

	chunks := tgtext.Split(r.Text, tgtext.MaxMessage)

	for i, chunk := range chunks {
		msg := entity.Message{Text: chunk}
		if i == len(chunks)-1 {
			msg.Keyboard = r.Keyboard
		}

		var err error
		if i == 0 && r.Edit && u.MessageID != 0 {
			err = c.sender.EditMessage(ctx, u.ChatID, u.MessageID, msg)
		} else {
			_, err = c.sender.SendMessage(ctx, u.ChatID, msg)
		}

		if err != nil {
			c.log.Error("send message", "err", err, "chat", u.ChatID)

			return
		}
	}
}

func (c *Controller) action(ctx context.Context, chatID int64, action string) {
	if err := c.sender.ChatAction(ctx, chatID, action); err != nil {
		c.log.Debug("chat action", "err", err)
	}
}

func (c *Controller) answer(ctx context.Context, callbackID, text string) {
	if err := c.sender.AnswerCallback(ctx, callbackID, text); err != nil {
		c.log.Debug("answer callback", "err", err)
	}
}

func (c *Controller) authorized(ctx context.Context, u entity.Update) bool {
	if c.opts.NoAllowlist {
		if !u.IsCallback() && c.limiter.Allow(fmt.Sprint("hint:", u.ChatID), chatIDHintEvery, c.clock.Now()) {
			text := fmt.Sprintf("Your chat id is %s. Add it to /etc/config/wrtgram:\n%s\nthen restart the bot.",
				tgtext.Code(fmt.Sprint(u.ChatID)), tgtext.Pre(fmt.Sprintf("uci add_list wrtgram.main.chat_id='%d'\nuci commit wrtgram", u.ChatID)))
			c.reply(ctx, u, module.Reply{Text: text})
		}

		return false
	}

	if c.opts.Allowed(u.ChatID) {
		return true
	}

	if c.limiter.Allow(fmt.Sprint("unauth:", u.ChatID), unauthWindow, c.clock.Now()) {
		c.log.Warn("ignoring update from a chat that is not allowed", "chat", u.ChatID, "user", u.Username)
	}

	return false
}

func (c *Controller) helpText() string {
	var b strings.Builder

	b.WriteString("<b>Commands</b>\n")

	for _, cmd := range c.MenuCommands() {
		fmt.Fprintf(&b, "/%s — %s\n", cmd.Name, tgtext.Esc(cmd.Description))
	}

	return strings.TrimRight(b.String(), "\n")
}

// parseCommand splits "/name@bot arg1 arg2" into its parts.
func parseCommand(text string) (name string, args []string, ok bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", nil, false
	}

	fields := strings.Fields(text[1:])
	if len(fields) == 0 {
		return "", nil, false
	}

	name, _, _ = strings.Cut(fields[0], "@")
	name = strings.ToLower(name)

	args = fields[1:]
	if len(args) > maxArgs {
		args = args[:maxArgs]
	}

	return name, args, name != ""
}

func chatHash(id int64) uint32 {
	h := fnv.New32a()
	_, _ = fmt.Fprint(h, id)

	return h.Sum32()
}
