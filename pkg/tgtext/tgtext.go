// Package tgtext builds Telegram HTML: escaping, the few allowed tags, and
// splitting long texts into messages that fit the 4096-character limit.
package tgtext

import "strings"

// MaxMessage is Telegram's limit on message text length in characters.
const MaxMessage = 4096

const (
	preOpen  = "<pre>"
	preClose = "</pre>"
)

// Esc escapes the characters Telegram's HTML parse mode requires.
func Esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// B wraps escaped text in bold.
func B(s string) string { return "<b>" + Esc(s) + "</b>" }

// I wraps escaped text in italics.
func I(s string) string { return "<i>" + Esc(s) + "</i>" }

// Code wraps escaped text in inline code.
func Code(s string) string { return "<code>" + Esc(s) + "</code>" }

// Pre wraps escaped text in a code block.
func Pre(s string) string { return preOpen + Esc(s) + preClose }
