package telegram

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	confirmTTL   = 10 * time.Minute
	nonceBytes   = 6
	confirmYes   = "yes"
	confirmNo    = "no"
	confirmLimit = 64
)

type pending struct {
	expires time.Time
	command string
	chatID  int64
}

// confirmStore remembers the nonces of outstanding Yes/No prompts.
type confirmStore struct {
	items map[string]pending
	mu    sync.Mutex
}

func newConfirmStore() *confirmStore { return &confirmStore{items: make(map[string]pending)} }

func (s *confirmStore) add(command string, chatID int64, now time.Time) string {
	buf := make([]byte, nonceBytes)
	_, _ = rand.Read(buf)
	nonce := hex.EncodeToString(buf)

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, p := range s.items {
		if now.After(p.expires) || len(s.items) > confirmLimit {
			delete(s.items, k)
		}
	}

	s.items[nonce] = pending{expires: now.Add(confirmTTL), command: command, chatID: chatID}

	return nonce
}

// take validates and consumes a nonce.
func (s *confirmStore) take(nonce, command string, chatID int64, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.items[nonce]
	if !ok {
		return false
	}

	delete(s.items, nonce)

	return p.command == command && p.chatID == chatID && now.Before(p.expires)
}
