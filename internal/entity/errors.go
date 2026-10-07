package entity

import (
	"fmt"
	"time"
)

// RateLimitError is returned by the transport when Telegram asks to slow down.
type RateLimitError struct {
	RetryAfter time.Duration
}

// Error implements error.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited, retry after %s", e.RetryAfter)
}
