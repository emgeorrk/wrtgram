//go:build unix

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

	bsize := uint64(st.Bsize)

	return st.Blocks * bsize, st.Bavail * bsize, nil
}
