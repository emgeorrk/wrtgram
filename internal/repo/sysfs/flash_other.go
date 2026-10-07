//go:build !linux && !darwin

package sysfs

import "errors"

var errStatfs = errors.New("statfs not supported on this platform")

// Flash is unavailable on this platform.
func Flash(string) (total, avail uint64, err error) { return 0, 0, errStatfs }
