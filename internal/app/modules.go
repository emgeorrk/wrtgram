package app

import (
	"context"

	"github.com/emgeorrk/wrtgram/internal/module"
	networkmod "github.com/emgeorrk/wrtgram/internal/module/network"
	systemmod "github.com/emgeorrk/wrtgram/internal/module/system"
	thermalmod "github.com/emgeorrk/wrtgram/internal/module/thermal"
	"github.com/emgeorrk/wrtgram/internal/repo/netifd"
	"github.com/emgeorrk/wrtgram/internal/repo/sysfs"
	"github.com/emgeorrk/wrtgram/internal/usecase/system"
)

// services are the usecases the modules and CLI share.
type services struct {
	system *system.Service
	wan    *netifd.WAN
}

func (e *env) buildServices() *services {
	wan := netifd.NewWAN(e.ubus, e.cfg.Main.WANInterface)

	return &services{
		wan:    wan,
		system: system.New(e.ubus, wan, e.fsys, sysfs.Flash, e.overlay),
	}
}

// buildRegistry activates every enabled and detected module.
func (e *env) buildRegistry(ctx context.Context, svc *services) (*module.Registry, error) {
	reg := module.NewRegistry(e.log)

	mods := []module.Module{
		systemmod.New(svc.system, e.log, e.cfg.NotificationEnabled("boot")),
		networkmod.New(svc.wan),
		thermalmod.New(svc.system),
	}

	for _, m := range mods {
		if err := reg.Add(ctx, m, e.cfg.ModuleEnabled(m.Name())); err != nil {
			return nil, err
		}
	}

	return reg, nil
}

// builtinNames reports whether a custom command would shadow a built-in one.
func builtinNames(reg *module.Registry) func(string) bool {
	return func(name string) bool {
		return name == module.HelpCommand || name == module.StartCommand || reg.Has(name)
	}
}
