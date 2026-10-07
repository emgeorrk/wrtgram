// Package throttle rate-limits events to one per key per window.
package throttle

import (
	"sync"
	"time"
)

const pruneAt = 1024

// Limiter remembers when each key last passed.
type Limiter struct {
	last map[string]time.Time
	mu   sync.Mutex
}

// New creates a Limiter.
func New() *Limiter { return &Limiter{last: make(map[string]time.Time)} }

// Allow reports whether key may fire now, i.e. the previous pass is older
// than window. A pass is recorded even when the caller's send later fails,
// which keeps a flapping event from retrying every tick.
func (l *Limiter) Allow(key string, window time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if t, ok := l.last[key]; ok && now.Sub(t) < window {
		return false
	}

	l.last[key] = now

	if len(l.last) > pruneAt {
		l.prune(window, now)
	}

	return true
}

// Reset forgets a key.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.last, key)
}

func (l *Limiter) prune(window time.Duration, now time.Time) {
	for k, t := range l.last {
		if now.Sub(t) >= window {
			delete(l.last, k)
		}
	}
}
