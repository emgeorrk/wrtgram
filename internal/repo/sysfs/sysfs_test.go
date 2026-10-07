package sysfs_test

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/emgeorrk/wrtgram/internal/repo/sysfs"
)

func TestReaders(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"proc/uptime":                        {Data: []byte("1234.56 2000.00\n")},
		"etc/os-release":                     {Data: []byte("NAME=\"OpenWrt\"\nOPENWRT_ARCH=\"mipsel_24kc\"\n")},
		"sys/class/hwmon/hwmon0/name":        {Data: []byte("cpu_thermal\n")},
		"sys/class/hwmon/hwmon0/temp1_input": {Data: []byte("66693\n")},
		"sys/class/hwmon/hwmon1/name":        {Data: []byte("mt7915_phy0\n")},
		"sys/class/hwmon/hwmon1/temp1_input": {Data: []byte("60000\n")},
		"sys/class/hwmon/hwmon1/temp1_label": {Data: []byte("wifi\n")},
		"sys/class/hwmon/hwmon2/name":        {Data: []byte("broken\n")},
		"sys/class/hwmon/hwmon2/temp1_input": {Data: []byte("garbage\n")},
	}

	up, err := sysfs.Uptime(fsys)
	if err != nil || up != 1234*time.Second+560*time.Millisecond {
		t.Errorf("Uptime = %v, %v", up, err)
	}

	if arch := sysfs.Arch(fsys); arch != "mipsel_24kc" {
		t.Errorf("Arch = %q", arch)
	}

	temps, err := sysfs.Temperatures(fsys)
	if err != nil {
		t.Fatalf("Temperatures: %v", err)
	}

	if len(temps) != 2 || temps[0].Sensor != "cpu_thermal" || temps[0].Celsius != 66.693 ||
		temps[1].Sensor != "mt7915_phy0" || temps[1].Label != "wifi" {
		t.Errorf("Temperatures = %+v", temps)
	}

	if _, err := sysfs.Temperatures(fstest.MapFS{}); err == nil {
		t.Error("no hwmon must be an error")
	}
}
