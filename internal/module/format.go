package module

import (
	"fmt"
	"strings"
	"time"
)

const (
	kib        = 1024
	hoursInDay = 24
)

// FormatDuration renders a duration as "3d 4h 5m" (or "4h 5m", "5m", "30s").
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}

	days := int(d.Hours()) / hoursInDay
	hours := int(d.Hours()) % hoursInDay
	mins := int(d.Minutes()) % int(time.Hour/time.Minute)

	var b strings.Builder

	if days > 0 {
		fmt.Fprintf(&b, "%dd ", days)
	}

	if days > 0 || hours > 0 {
		fmt.Fprintf(&b, "%dh ", hours)
	}

	fmt.Fprintf(&b, "%dm", mins)

	return b.String()
}

// FormatBytes renders a byte count with a binary unit, e.g. "38.4 MiB".
func FormatBytes(n uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	v := float64(n)
	i := 0

	for v >= kib && i < len(units)-1 {
		v /= kib
		i++
	}

	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}

	return fmt.Sprintf("%.1f %s", v, units[i])
}
