// Package backup creates configuration archives, optionally encrypted in
// the openssl enc format so they can be opened anywhere.
package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
	"github.com/emgeorrk/wrtgram/pkg/osslenc"
)

var errNoBackuper = errors.New("sysupgrade not available")

const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Service is the backup usecase.
type Service struct {
	backuper usecase.Backuper
	clock    usecase.Clock
	dir      string
	password string
}

// New creates the service. dir holds temporary archives (a tmpfs is fine).
func New(b usecase.Backuper, clock usecase.Clock, dir, password string) *Service {
	return &Service{backuper: b, clock: clock, dir: dir, password: password}
}

// Encrypted reports whether a password is configured.
func (s *Service) Encrypted() bool { return s.password != "" }

// Create produces the archive as a document ready to send. cleanup removes
// the temporary files and must be called after the upload.
func (s *Service) Create(ctx context.Context, hostname string) (*entity.Document, func(), error) {
	if s.backuper == nil {
		return nil, nil, errNoBackuper
	}

	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return nil, nil, fmt.Errorf("backup dir: %w", err)
	}

	stamp := s.clock.Now().Format("20060102-1504")
	base := filepath.Join(s.dir, fmt.Sprintf("backup-%s-%s.tar.gz", hostname, stamp))

	cleanup := func() {
		os.Remove(base)
		os.Remove(base + ".enc")
	}

	if err := s.backuper.Backup(ctx, base); err != nil {
		cleanup()

		return nil, nil, err
	}

	path, caption, err := s.finish(base, stamp)
	if err != nil {
		cleanup()

		return nil, nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		cleanup()

		return nil, nil, fmt.Errorf("open backup: %w", err)
	}

	doc := &entity.Document{Data: f, Name: filepath.Base(path), Caption: caption}

	return doc, func() { f.Close(); cleanup() }, nil
}

// finish encrypts the archive when a password is set and builds the caption.
func (s *Service) finish(base, stamp string) (path, caption string, err error) {
	date := stamp[:len("20060102")]

	if s.password == "" {
		return base, fmt.Sprintf("Config backup %s. Unencrypted: it contains Wi-Fi and VPN keys, do not forward.", date), nil
	}

	if err := encryptFile(base, base+".enc", s.password); err != nil {
		return "", "", err
	}

	caption = fmt.Sprintf("Config backup %s, encrypted with the backup password.\nDecrypt: openssl enc -d -aes-256-cbc -pbkdf2 -iter %d -in FILE.enc -out FILE",
		date, osslenc.DefaultIter)

	return base + ".enc", caption, nil
}

func encryptFile(src, dst, password string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("create encrypted archive: %w", err)
	}
	defer out.Close()

	if err := osslenc.Encrypt(out, in, password, osslenc.DefaultIter); err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	return nil
}

// Age is a helper for schedulers: time since a stamp.
func Age(now, last time.Time) time.Duration { return now.Sub(last) }
