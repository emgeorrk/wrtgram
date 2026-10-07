//go:build linux

package sysfs

import (
	"errors"
	"fmt"
	"syscall"
)

var errStatfs = errors.New("statfs failed")

// Flash returns total and available bytes of the filesystem holding path
// (the overlay on OpenWrt). It is the only syscall in the package.
func Flash(path string) (total, avail uint64, err error) {
	var st syscall.Statfs_t

	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("%w: %s: %w", errStatfs, path, err)
	}

	if st.Bsize <= 0 {
		return 0, 0, fmt.Errorf("%w: %s: block size %d", errStatfs, path, st.Bsize)
	}

	bsize := uint64(st.Bsize) //nolint:gosec // checked non-negative above

	return st.Blocks * bsize, st.Bavail * bsize, nil
}
