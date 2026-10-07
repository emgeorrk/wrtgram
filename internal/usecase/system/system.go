// Package system gathers the facts behind /status: board, uptime, load,
// memory, flash, temperatures and the WAN link.
package system

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/repo/sysfs"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

const loadScale = 65536.0

// FlashFunc reports total and available bytes of the overlay.
type FlashFunc func(path string) (total, avail uint64, err error)

// Service is the system usecase.
type Service struct {
	ubus    usecase.Ubus
	wan     usecase.WAN
	fsys    fs.FS
	flash   FlashFunc
	overlay string
}

// New creates the service. wan may be nil when the network module is off.
func New(ubus usecase.Ubus, wan usecase.WAN, fsys fs.FS, flash FlashFunc, overlay string) *Service {
	return &Service{ubus: ubus, wan: wan, fsys: fsys, flash: flash, overlay: overlay}
}

// Snapshot is everything /status shows. Optional parts carry their own error
// so one missing source never hides the rest.
type Snapshot struct {
	WANErr   error
	TempsErr error
	Board    entity.Board
	Temps    []entity.Temperature
	WAN      entity.WANStatus
	Info     entity.SystemInfo
}

type boardReply struct {
	Kernel   string `json:"kernel"`
	Hostname string `json:"hostname"`
	Model    string `json:"model"`
	Release  struct {
		Target      string `json:"target"`
		Description string `json:"description"`
	} `json:"release"`
}

type infoReply struct {
	Uptime int64     `json:"uptime"`
	Load   [3]uint64 `json:"load"`
	Memory struct {
		Total     uint64 `json:"total"`
		Available uint64 `json:"available"`
	} `json:"memory"`
}

// Board reads the hardware and firmware description.
func (s *Service) Board(ctx context.Context) (entity.Board, error) {
	var r boardReply
	if err := s.ubus.Call(ctx, "system", "board", nil, &r); err != nil {
		return entity.Board{}, fmt.Errorf("system board: %w", err)
	}

	return entity.Board{
		Model:    r.Model,
		Hostname: r.Hostname,
		Release:  r.Release.Description,
		Kernel:   r.Kernel,
		Target:   r.Release.Target,
		Arch:     sysfs.Arch(s.fsys),
	}, nil
}

// Info reads uptime, load, memory and flash usage.
func (s *Service) Info(ctx context.Context) (entity.SystemInfo, error) {
	var r infoReply
	if err := s.ubus.Call(ctx, "system", "info", nil, &r); err != nil {
		return entity.SystemInfo{}, fmt.Errorf("system info: %w", err)
	}

	info := entity.SystemInfo{
		Uptime:   time.Duration(r.Uptime) * time.Second,
		MemTotal: r.Memory.Total,
		MemAvail: r.Memory.Available,
	}

	for i, l := range r.Load {
		info.Load[i] = float64(l) / loadScale
	}

	if s.flash != nil {
		if total, avail, err := s.flash(s.overlay); err == nil {
			info.FlashTotal, info.FlashFree = total, avail
		}
	}

	return info, nil
}

// Temperatures reads the hwmon sensors.
func (s *Service) Temperatures() ([]entity.Temperature, error) {
	return sysfs.Temperatures(s.fsys)
}

// Snapshot collects everything for /status.
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	board, err := s.Board(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	info, err := s.Info(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	snap := Snapshot{Board: board, Info: info}
	snap.Temps, snap.TempsErr = s.Temperatures()

	if s.wan != nil {
		snap.WAN, snap.WANErr = s.wan.Status(ctx)
	}

	return snap, nil
}

// Reboot asks procd to reboot the router.
func (s *Service) Reboot(ctx context.Context) error {
	if err := s.ubus.Call(ctx, "system", "reboot", nil, nil); err != nil {
		return fmt.Errorf("system reboot: %w", err)
	}

	return nil
}
