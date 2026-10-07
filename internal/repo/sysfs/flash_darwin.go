//go:build darwin

package sysfs

import (
	"errors"
	"fmt"
	"syscall"
)

var errStatfs = errors.New("statfs failed")

// Flash returns total and available bytes of the filesystem holding path.
// The development host only; the router build uses flash_linux.go.
func Flash(path string) (total, avail uint64, err error) {
	var st syscall.Statfs_t

	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("%w: %s: %w", errStatfs, path, err)
	}

	bsize := uint64(st.Bsize)

	return st.Blocks * bsize, st.Bavail * bsize, nil
}
