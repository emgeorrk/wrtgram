package app

import "errors"

var (
	errUnknownCommand = errors.New("unknown command")
	errUsage          = errors.New("usage")
	errConfig         = errors.New("configuration")
	errNoFixtures     = errors.New("WRTGRAM_FAKE=1 needs a fixture file")
)
