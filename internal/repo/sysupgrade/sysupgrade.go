// Package sysupgrade creates configuration archives with `sysupgrade -b`.
package sysupgrade

import (
	"context"
	"fmt"

	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// Backuper implements usecase.Backuper.
type Backuper struct {
	run usecase.Runner
}

// New creates the adapter.
func New(run usecase.Runner) *Backuper { return &Backuper{run: run} }

// Available reports whether sysupgrade exists.
func (b *Backuper) Available() bool { return b.run.LookPath("sysupgrade") }

// Backup implements usecase.Backuper.
func (b *Backuper) Backup(ctx context.Context, path string) error {
	if _, err := b.run.Run(ctx, "sysupgrade", "-b", path); err != nil {
		return fmt.Errorf("sysupgrade -b: %w", err)
	}

	return nil
}
