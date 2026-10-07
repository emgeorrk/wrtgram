package entity

import (
	"io"
	"time"
)

// Update is an incoming Telegram update reduced to what the bot needs.
type Update struct {
	At           time.Time
	Text         string // message text or "" for callbacks
	CallbackID   string // non-empty for callback queries
	CallbackData string
	Username     string
	ChatID       int64
	UserID       int64
	UpdateID     int64
	MessageID    int
	Private      bool // private chat with the bot (not a group)
}

// IsCallback reports whether the update is an inline-button press.
func (u Update) IsCallback() bool { return u.CallbackID != "" }

// Button is one inline keyboard button; Data is the callback payload (≤ 64 bytes).
type Button struct {
	Text string
	Data string
}

// Message is an outgoing text message (HTML parse mode).
type Message struct {
	Text     string
	Keyboard [][]Button
}

// Document is an outgoing file.
type Document struct {
	Data    io.Reader
	Name    string
	Caption string
}

// BotCommand is one entry of the command menu (setMyCommands).
type BotCommand struct {
	Name        string
	Description string
}

// BotInfo is the result of getMe.
type BotInfo struct {
	Username string
	ID       int64
}
