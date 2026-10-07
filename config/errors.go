package config

import "errors"

var (
	errNoToken        = errors.New("main.token is not set")
	errBadToken       = errors.New("main.token does not look like a bot token")
	errBadChatID      = errors.New("chat_id is not a number")
	errBadProxy       = errors.New("main.proxy must be http://, https://, socks5:// or socks5h://")
	errBadLogLevel    = errors.New("main.log_level must be debug, info, warn or error")
	errBadCommandName = errors.New("command name must match [a-z0-9_]{1,32}")
	errEmptyCommand   = errors.New("command has no 'command' option")
	errDuplicate      = errors.New("duplicate command name")
)
