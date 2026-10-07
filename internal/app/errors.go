package app

import "errors"

var (
	errUnknownCommand = errors.New("unknown command")
	errNotImplemented = errors.New("not implemented yet")
	errConfig         = errors.New("configuration")
	errNoFixtures     = errors.New("WRTGRAM_FAKE=1 needs a fixture file")
)
