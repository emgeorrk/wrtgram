package telegram

import "errors"

var (
	errAPI      = errors.New("telegram api")
	errProxyURL = errors.New("invalid proxy url")
	errInit     = errors.New("telegram client init")
)
