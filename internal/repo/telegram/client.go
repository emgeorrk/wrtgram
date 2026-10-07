// Package telegram adapts github.com/go-telegram/bot to the Sender and
// UpdateSource ports. Library types never leave this package, and every
// error is scrubbed of the bot token before it is returned.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	pollTimeout      = 50 * time.Second
	clientTimeout    = 90 * time.Second // > pollTimeout, and enough for a backup upload
	handshakeTimeout = 20 * time.Second
	updatesCap       = 32
	idleConns        = 2
	mask             = "***"
)

// Client implements usecase.Sender and usecase.UpdateSource.
type Client struct {
	bot     *bot.Bot
	log     *slog.Logger
	updates chan entity.Update
	token   string
	start   sync.Once
}

// New creates a client. proxy is an http(s):// or socks5(h):// URL or "".
func New(token, proxy string, log *slog.Logger) (*Client, error) {
	transport, err := newTransport(proxy)
	if err != nil {
		return nil, err
	}

	c := &Client{log: log, updates: make(chan entity.Update, updatesCap), token: token}

	b, err := bot.New(token,
		bot.WithSkipGetMe(), // the clock may be wrong at boot; getMe is retried by the app
		bot.WithHTTPClient(pollTimeout, &http.Client{Timeout: clientTimeout, Transport: transport}),
		bot.WithAllowedUpdates(bot.AllowedUpdates{"message", "callback_query"}),
		bot.WithDefaultHandler(c.onUpdate),
		bot.WithNotAsyncHandlers(),
		bot.WithErrorsHandler(c.onError),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInit, c.scrub(err))
	}

	c.bot = b

	return c, nil
}

func newTransport(proxy string) (*http.Transport, error) {
	t := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: handshakeTimeout,
		MaxIdleConns:        idleConns,
		IdleConnTimeout:     pollTimeout,
	}

	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errProxyURL, err)
		}

		t.Proxy = http.ProxyURL(u)
	}

	return t, nil
}

// Updates starts long polling on first use and returns the update stream.
// The channel closes when ctx is done.
func (c *Client) Updates(ctx context.Context) <-chan entity.Update {
	c.start.Do(func() {
		go func() {
			c.bot.Start(ctx)
			close(c.updates)
		}()
	})

	return c.updates
}

func (c *Client) onUpdate(ctx context.Context, _ *bot.Bot, u *models.Update) {
	upd, ok := toUpdate(u)
	if !ok {
		return
	}

	select {
	case c.updates <- upd:
	case <-ctx.Done():
	}
}

func (c *Client) onError(err error) {
	if errors.Is(err, context.Canceled) {
		return
	}

	c.log.Warn("telegram polling", "err", c.scrub(err))
}

func toUpdate(u *models.Update) (entity.Update, bool) {
	switch {
	case u.Message != nil:
		m := u.Message

		out := entity.Update{
			At:        time.Unix(int64(m.Date), 0),
			Text:      m.Text,
			ChatID:    m.Chat.ID,
			UpdateID:  u.ID,
			MessageID: m.ID,
			Private:   m.Chat.Type == models.ChatTypePrivate,
		}

		if m.From != nil {
			out.UserID, out.Username = m.From.ID, m.From.Username
		}

		return out, true
	case u.CallbackQuery != nil && u.CallbackQuery.Message.Message != nil:
		cq := u.CallbackQuery
		m := cq.Message.Message

		return entity.Update{
			At:           time.Now(),
			CallbackID:   cq.ID,
			CallbackData: cq.Data,
			Username:     cq.From.Username,
			ChatID:       m.Chat.ID,
			UserID:       cq.From.ID,
			UpdateID:     u.ID,
			MessageID:    m.ID,
			Private:      m.Chat.Type == models.ChatTypePrivate,
		}, true
	}

	return entity.Update{}, false
}

// Me implements usecase.Sender.
func (c *Client) Me(ctx context.Context) (entity.BotInfo, error) {
	u, err := c.bot.GetMe(ctx)
	if err != nil {
		return entity.BotInfo{}, c.wrap(err)
	}

	return entity.BotInfo{Username: u.Username, ID: u.ID}, nil
}

// SendMessage implements usecase.Sender.
func (c *Client) SendMessage(ctx context.Context, chatID int64, msg entity.Message) (int, error) {
	params := &bot.SendMessageParams{
		ChatID:             chatID,
		Text:               msg.Text,
		ParseMode:          models.ParseModeHTML,
		LinkPreviewOptions: noPreview(),
		ReplyMarkup:        keyboard(msg.Keyboard),
	}

	m, err := c.bot.SendMessage(ctx, params)
	if err != nil {
		return 0, c.wrap(err)
	}

	return m.ID, nil
}

// EditMessage implements usecase.Sender.
func (c *Client) EditMessage(ctx context.Context, chatID int64, messageID int, msg entity.Message) error {
	params := &bot.EditMessageTextParams{
		ChatID:             chatID,
		MessageID:          messageID,
		Text:               msg.Text,
		ParseMode:          models.ParseModeHTML,
		LinkPreviewOptions: noPreview(),
		ReplyMarkup:        keyboard(msg.Keyboard),
	}

	if _, err := c.bot.EditMessageText(ctx, params); err != nil {
		return c.wrap(err)
	}

	return nil
}

// SendDocument implements usecase.Sender.
func (c *Client) SendDocument(ctx context.Context, chatID int64, doc entity.Document) error {
	params := &bot.SendDocumentParams{
		ChatID:   chatID,
		Document: &models.InputFileUpload{Filename: doc.Name, Data: doc.Data},
		Caption:  doc.Caption,
	}

	if _, err := c.bot.SendDocument(ctx, params); err != nil {
		return c.wrap(err)
	}

	return nil
}

// AnswerCallback implements usecase.Sender.
func (c *Client) AnswerCallback(ctx context.Context, callbackID, text string) error {
	_, err := c.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callbackID, Text: text})
	if err != nil {
		return c.wrap(err)
	}

	return nil
}

// SetCommands implements usecase.Sender.
func (c *Client) SetCommands(ctx context.Context, cmds []entity.BotCommand) error {
	list := make([]models.BotCommand, 0, len(cmds))
	for _, cmd := range cmds {
		list = append(list, models.BotCommand{Command: cmd.Name, Description: cmd.Description})
	}

	if _, err := c.bot.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: list}); err != nil {
		return c.wrap(err)
	}

	return nil
}

// ChatAction implements usecase.Sender.
func (c *Client) ChatAction(ctx context.Context, chatID int64, action string) error {
	a := models.ChatActionTyping
	if action == "upload_document" {
		a = models.ChatActionUploadDocument
	}

	if _, err := c.bot.SendChatAction(ctx, &bot.SendChatActionParams{ChatID: chatID, Action: a}); err != nil {
		return c.wrap(err)
	}

	return nil
}

func keyboard(rows [][]entity.Button) models.ReplyMarkup {
	if len(rows) == 0 {
		return nil
	}

	kb := make([][]models.InlineKeyboardButton, 0, len(rows))

	for _, row := range rows {
		r := make([]models.InlineKeyboardButton, 0, len(row))
		for _, b := range row {
			r = append(r, models.InlineKeyboardButton{Text: b.Text, CallbackData: b.Data})
		}

		kb = append(kb, r)
	}

	return &models.InlineKeyboardMarkup{InlineKeyboard: kb}
}

func noPreview() *models.LinkPreviewOptions {
	disabled := true

	return &models.LinkPreviewOptions{IsDisabled: &disabled}
}

// wrap converts library errors: rate limits become entity.RateLimitError,
// context errors pass through, everything else is scrubbed of the token.
func (c *Client) wrap(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	var tooMany *bot.TooManyRequestsError
	if errors.As(err, &tooMany) {
		return &entity.RateLimitError{RetryAfter: time.Duration(tooMany.RetryAfter) * time.Second}
	}

	return fmt.Errorf("%w: %w", errAPI, c.scrub(err))
}

// scrub returns an error whose text no longer contains the token. The
// library wraps *url.Error, whose message carries the request URL.
func (c *Client) scrub(err error) error {
	msg := err.Error()
	if !strings.Contains(msg, c.token) {
		return err
	}

	return errors.New(strings.ReplaceAll(msg, c.token, mask)) //nolint:err113 // deliberately rebuilt without the secret
}
