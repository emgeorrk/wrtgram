package config

import (
	"fmt"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/module"
)

const minTokenSecret = 30

// Warning is a non-fatal configuration problem.
type Warning string

// Validate checks the configuration. The error is fatal for `run`; warnings
// are printed by check-config and logged at startup.
func (c Config) Validate(builtin func(name string) bool) ([]Warning, error) {
	var warns []Warning

	if c.Main.Token == "" {
		return nil, errNoToken
	}

	if !ValidToken(c.Main.Token) {
		return nil, errBadToken
	}

	if err := validateProxy(c.Main.Proxy); err != nil {
		return nil, err
	}

	if len(c.Main.ChatIDs) == 0 {
		warns = append(warns, "main.chat_id is empty: the bot will only tell you your chat id")
	}

	seen := make(map[string]bool, len(c.Commands))

	for _, cmd := range c.Commands {
		if !module.ValidCommandName(cmd.Name) {
			return nil, fmt.Errorf("%w: %q", errBadCommandName, cmd.Name)
		}

		if seen[cmd.Name] || (builtin != nil && builtin(cmd.Name)) {
			return nil, fmt.Errorf("%w: /%s", errDuplicate, cmd.Name)
		}

		seen[cmd.Name] = true
	}

	return warns, nil
}

// ValidToken reports whether s looks like "<digits>:<secret>", the BotFather
// token shape, without accepting the secret itself as proof.
func ValidToken(s string) bool {
	id, secret, ok := strings.Cut(s, ":")

	return ok && id != "" && allDigits(id) && len(secret) >= minTokenSecret && allTokenChars(secret)
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

func allTokenChars(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}

	return true
}

func validateProxy(p string) error {
	if p == "" {
		return nil
	}

	for _, scheme := range []string{"http://", "https://", "socks5://", "socks5h://"} {
		if strings.HasPrefix(p, scheme) {
			return nil
		}
	}

	return fmt.Errorf("%w: %q", errBadProxy, p)
}
