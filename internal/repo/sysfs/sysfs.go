// Package sysfs reads /proc and /sys through an fs.FS rooted at "/", so tests
// and the host dev mode can substitute a directory or an fstest.MapFS.
package sysfs

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
)

const (
	hwmonDir   = "sys/class/hwmon"
	uptimeFile = "proc/uptime"
	osRelease  = "etc/os-release"
	milli      = 1000
)

var (
	errUptime  = errors.New("proc/uptime unreadable")
	errNoHwmon = errors.New("no hwmon temperature sensors")
)

// Uptime reads the system uptime.
func Uptime(fsys fs.FS) (time.Duration, error) {
	raw, err := fs.ReadFile(fsys, uptimeFile)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errUptime, err)
	}

	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0, errUptime
	}

	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errUptime, err)
	}

	return time.Duration(secs * float64(time.Second)), nil
}

// Temperatures reads every hwmon temp*_input. Sensors are identified by the
// hwmon name (and temp*_label when present) because hwmon numbers change
// between boots.
func Temperatures(fsys fs.FS) ([]entity.Temperature, error) {
	dirs, err := fs.ReadDir(fsys, hwmonDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoHwmon, err)
	}

	var out []entity.Temperature

	for _, d := range dirs {
		out = append(out, readHwmon(fsys, path.Join(hwmonDir, d.Name()))...)
	}

	if len(out) == 0 {
		return nil, errNoHwmon
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Sensor < out[j].Sensor })

	return out, nil
}

func readHwmon(fsys fs.FS, dir string) []entity.Temperature {
	name := readString(fsys, path.Join(dir, "name"))

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}

	var out []entity.Temperature

	for _, e := range entries {
		base := e.Name()
		if !strings.HasPrefix(base, "temp") || !strings.HasSuffix(base, "_input") {
			continue
		}

		raw := readString(fsys, path.Join(dir, base))

		mdeg, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}

		label := readString(fsys, path.Join(dir, strings.TrimSuffix(base, "_input")+"_label"))

		out = append(out, entity.Temperature{Sensor: name, Label: label, Celsius: float64(mdeg) / milli})
	}

	return out
}

// Arch returns OPENWRT_ARCH from /etc/os-release ("" when absent).
func Arch(fsys fs.FS) string {
	raw, err := fs.ReadFile(fsys, osRelease)
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, "OPENWRT_ARCH="); ok {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}

	return ""
}

func readString(fsys fs.FS, p string) string {
	raw, err := fs.ReadFile(fsys, p)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(raw))
}
