package module

import "errors"

var (
	errDuplicateCommand = errors.New("duplicate command")
	errBadCommandName   = errors.New("invalid command name")
)
