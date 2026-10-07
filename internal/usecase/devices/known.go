package devices

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	knownDirMode  = 0o700
	knownFileMode = 0o600
)

var errKnownFile = errors.New("known devices file")

// Known is the persistent set of MAC addresses already seen on the LAN.
// A missing file means a fresh install (the current devices get seeded
// silently); an existing empty file means the user forgot everything and
// wants a card for each device that shows up.
type Known struct {
	macs  map[string]bool
	path  string
	mu    sync.Mutex
	fresh bool
}

// NewKnown loads the file.
func NewKnown(path string) (*Known, error) {
	k := &Known{macs: make(map[string]bool), path: path}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			k.fresh = true

			return k, nil
		}

		return nil, fmt.Errorf("%w: %w", errKnownFile, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if mac := strings.ToLower(strings.TrimSpace(sc.Text())); mac != "" {
			k.macs[mac] = true
		}
	}

	return k, nil
}

// Fresh reports whether no file existed at load time.
func (k *Known) Fresh() bool { return k.fresh }

// Len returns the number of known addresses.
func (k *Known) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()

	return len(k.macs)
}

// Seed records addresses without reporting them (first run).
func (k *Known) Seed(macs []string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	for _, m := range macs {
		k.macs[strings.ToLower(m)] = true
	}

	k.fresh = false

	return k.save()
}

// Observe records mac and reports whether it was new.
func (k *Known) Observe(mac string) (bool, error) {
	mac = strings.ToLower(strings.TrimSpace(mac))

	k.mu.Lock()
	defer k.mu.Unlock()

	if mac == "" || k.macs[mac] {
		return false, nil
	}

	k.macs[mac] = true

	return true, k.save()
}

func (k *Known) save() error {
	if err := os.MkdirAll(filepath.Dir(k.path), knownDirMode); err != nil {
		return fmt.Errorf("%w: %w", errKnownFile, err)
	}

	var b strings.Builder
	for m := range k.macs {
		b.WriteString(m + "\n")
	}

	tmp := k.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), knownFileMode); err != nil {
		return fmt.Errorf("%w: %w", errKnownFile, err)
	}

	if err := os.Rename(tmp, k.path); err != nil {
		return fmt.Errorf("%w: %w", errKnownFile, err)
	}

	return nil
}
