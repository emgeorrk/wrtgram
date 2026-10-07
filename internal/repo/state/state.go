// Package state persists small JSON documents atomically (tmp + rename).
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotFound is returned by Load when nothing was saved under that name.
var ErrNotFound = errors.New("state not found")

const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Store implements usecase.StateStore inside one directory.
type Store struct {
	dir string
}

// New creates a store in dir (created on first Save).
func New(dir string) *Store { return &Store{dir: dir} }

// Load implements usecase.StateStore.
func (s *Store) Load(name string, v any) error {
	raw, err := os.ReadFile(s.path(name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}

		return fmt.Errorf("state %s: %w", name, err)
	}

	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("state %s: %w", name, err)
	}

	return nil
}

// Save implements usecase.StateStore.
func (s *Store) Save(name string, v any) error {
	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return fmt.Errorf("state dir: %w", err)
	}

	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("state %s: %w", name, err)
	}

	tmp := s.path(name) + ".tmp"
	if err := os.WriteFile(tmp, raw, fileMode); err != nil {
		return fmt.Errorf("state %s: %w", name, err)
	}

	if err := os.Rename(tmp, s.path(name)); err != nil {
		return fmt.Errorf("state %s: %w", name, err)
	}

	return nil
}

func (s *Store) path(name string) string { return filepath.Join(s.dir, name+".json") }
