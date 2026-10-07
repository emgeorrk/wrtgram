package app

import (
	"context"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/emgeorrk/wrtgram/config"
	"github.com/emgeorrk/wrtgram/internal/module"
	backupmod "github.com/emgeorrk/wrtgram/internal/module/backup"
	custommod "github.com/emgeorrk/wrtgram/internal/module/custom"
	devicesmod "github.com/emgeorrk/wrtgram/internal/module/devices"
	failovermod "github.com/emgeorrk/wrtgram/internal/module/failover"
	loginsmod "github.com/emgeorrk/wrtgram/internal/module/logins"
	networkmod "github.com/emgeorrk/wrtgram/internal/module/network"
	servicesmod "github.com/emgeorrk/wrtgram/internal/module/services"
	systemmod "github.com/emgeorrk/wrtgram/internal/module/system"
	thermalmod "github.com/emgeorrk/wrtgram/internal/module/thermal"
	vpnmod "github.com/emgeorrk/wrtgram/internal/module/vpn"
	"github.com/emgeorrk/wrtgram/internal/repo/dhcp"
	"github.com/emgeorrk/wrtgram/internal/repo/iproute"
	"github.com/emgeorrk/wrtgram/internal/repo/logread"
	"github.com/emgeorrk/wrtgram/internal/repo/netifd"
	"github.com/emgeorrk/wrtgram/internal/repo/state"
	"github.com/emgeorrk/wrtgram/internal/repo/sysfs"
	"github.com/emgeorrk/wrtgram/internal/repo/sysupgrade"
	"github.com/emgeorrk/wrtgram/internal/repo/wg"
	"github.com/emgeorrk/wrtgram/internal/repo/wifi"
	"github.com/emgeorrk/wrtgram/internal/usecase"
	"github.com/emgeorrk/wrtgram/internal/usecase/backup"
	"github.com/emgeorrk/wrtgram/internal/usecase/custom"
	"github.com/emgeorrk/wrtgram/internal/usecase/devices"
	"github.com/emgeorrk/wrtgram/internal/usecase/failover"
	"github.com/emgeorrk/wrtgram/internal/usecase/logwatch"
	svcuc "github.com/emgeorrk/wrtgram/internal/usecase/services"
	"github.com/emgeorrk/wrtgram/internal/usecase/system"
	"github.com/emgeorrk/wrtgram/internal/usecase/thermal"
	"github.com/emgeorrk/wrtgram/internal/usecase/vpn"
)

// Paths on the router.
const (
	persistDir  = "/etc/wrtgram"     // survives reboots and sysupgrade (listed in conffiles)
	volatileDir = "/var/run/wrtgram" // tmpfs: failover state, temporary backups
	legacyGlob  = "etc/rc.d/S*vpn-failover"
)

// Defaults of the notification rules.
const (
	failoverGrace    = 300
	failoverInterval = 30
	loginsSuccessWin = 3600
	loginsFailureWin = 600
	thermalHigh      = 85
	thermalNormal    = 75
	thermalInterval  = 60
	thermalRemind    = 3600
	backupDay        = "sun"
	backupTime       = "03:30"
	knownMACsFile    = "known_macs"
)

// realClock implements usecase.Clock with the wall clock.
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// services are the usecases the modules and CLI share.
type services struct {
	system   *system.Service
	wan      *netifd.WAN
	tunnels  *wg.Tunnels
	leases   *dhcp.Leases
	wifi     *wifi.Hostapd
	devices  *devices.Service
	devmod   *devicesmod.Module // receives hotplug events through IPC
	failover *failover.Loop
	vpn      *vpn.Service
	backup   *backup.Service
	backuper *sysupgrade.Backuper
	custom   *custom.Executor
	services *svcuc.Service
	clock    usecase.Clock
}

func (e *env) buildServices(ctx context.Context) (*services, error) {
	clock := realClock{}
	wan := netifd.NewWAN(e.ubus, e.cfg.Main.WANInterface)
	tunnels := wg.New(e.run, e.ubus)
	leases := dhcp.NewLeases(e.fsys, e.leaseFile(ctx))
	hostapd := wifi.New(e.ubus)
	backuper := sysupgrade.New(e.run)

	s := &services{
		system:   system.New(e.ubus, wan, e.fsys, sysfs.Flash, e.overlay),
		wan:      wan,
		tunnels:  tunnels,
		leases:   leases,
		wifi:     hostapd,
		devices:  devices.New(leases, hostapd, dhcp.NewHints(e.ubus), clock),
		backup:   backup.New(backuper, clock, e.volatile(), e.cfg.Module(backupmod.Name).Opt("password", "")),
		backuper: backuper,
		custom:   custom.New(e.run),
		services: svcuc.New(e.ubus, e.run, e.cfg.Module(servicesmod.Name).List("service")),
		clock:    clock,
	}

	loop, err := e.buildFailover(wan, clock)
	if err != nil {
		return nil, err
	}

	s.failover = loop

	var fo vpn.Failover
	if loop != nil {
		fo = loop
	}

	s.vpn = vpn.New(tunnels, e.ubus, fo)

	return s, nil
}

// buildFailover returns nil when the module is off, unconfigured or when
// the legacy shell controller is still enabled (both own the same routes).
func (e *env) buildFailover(wan *netifd.WAN, clock usecase.Clock) (*failover.Loop, error) {
	sec := e.cfg.Module(failovermod.Name)
	tunnels := sec.List("tunnel")

	if !e.cfg.ModuleEnabled(failovermod.Name) || len(tunnels) == 0 {
		return nil, nil //nolint:nilnil // nil loop = module absent
	}

	if m, err := fs.Glob(e.fsys, legacyGlob); err == nil && len(m) > 0 {
		e.log.Warn("failover module disabled: the legacy vpn-failover service is still enabled", "init", m[0])

		return nil, nil //nolint:nilnil // nil loop = module absent
	}

	rules := failover.Rules{
		Tunnels:     tunnels,
		Targets:     listOr(sec.List("target"), []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}),
		ServiceNets: listOr(sec.List("service_net"), config.TelegramNets),
		Grace:       time.Duration(sec.Int("grace", failoverGrace)) * time.Second,
		Interval:    time.Duration(sec.Int("interval", failoverInterval)) * time.Second,
		NTPDirect:   sec.Bool("ntp_direct", true),
	}

	return failover.NewLoop(rules, failover.Deps{
		Pinger:  iproute.NewPinger(e.run),
		Router:  iproute.NewRouter(e.run, e.log),
		WAN:     wan,
		Store:   state.New(e.volatile()),
		Persist: state.New(e.persist()),
		Clock:   clock,
		Log:     e.log,
	})
}

// buildRegistry activates every enabled and detected module.
func (e *env) buildRegistry(ctx context.Context, svc *services) (*module.Registry, error) {
	reg := module.NewRegistry(e.log)

	sys := systemmod.New(svc.system, e.log, e.cfg.NotificationEnabled("boot"))
	vpnm := vpnmod.New(svc.vpn, svc.clock, func(ctx context.Context) bool {
		refs, err := svc.tunnels.List(ctx)

		return svc.tunnels.Available() && err == nil && len(refs) > 0
	})

	sys.AddStatusSection(vpnm.StatusSection)

	hostname := func(ctx context.Context) string {
		if b, err := svc.system.Board(ctx); err == nil && b.Hostname != "" {
			return b.Hostname
		}

		return "router"
	}

	svc.devmod = devicesmod.New(svc.devices, e.knownDevices(), func(ctx context.Context) bool {
		ifaces, err := svc.wifi.Interfaces(ctx)

		return svc.leases.Exists() || (err == nil && len(ifaces) > 0)
	}, e.cfg.Module(devicesmod.Name).Int("page_size", devicesmod.DefaultPageSize), e.log)

	tailer := logread.New(e.run, svc.clock, time.Local, e.log)

	mods := []module.Module{
		sys,
		networkmod.New(svc.wan),
		svc.devmod,
		vpnm,
		failovermod.New(svc.failover, svc.clock, e.cfg.NotificationEnabled("failover")),
		backupmod.New(svc.backup, e.backupScheduler(svc, hostname), hostname, func(context.Context) bool { return svc.backuper.Available() }),
		thermalmod.New(svc.system, e.thermalWatcher(svc)),
		loginsmod.New(e.loginWatcher(tailer, svc.clock), func(context.Context) bool { return tailer.Available() }),
		servicesmod.New(svc.services),
		custommod.New(svc.custom, e.cfg.Commands),
	}

	for _, m := range mods {
		if err := reg.Add(ctx, m, e.cfg.ModuleEnabled(m.Name())); err != nil {
			return nil, err
		}
	}

	return reg, nil
}

// knownDevices returns the known-MAC set, or nil when the rule is off.
func (e *env) knownDevices() *devices.Known {
	sec := e.cfg.Notification("new_device")
	if !sec.Bool("enabled", true) {
		return nil
	}

	path := sec.Opt("known_file", filepath.Join(e.persist(), knownMACsFile))

	known, err := devices.NewKnown(path)
	if err != nil {
		e.log.Warn("new-device notifications disabled", "err", err)

		return nil
	}

	return known
}

func (e *env) loginWatcher(tailer *logread.Tailer, clock usecase.Clock) *logwatch.Watcher {
	sec := e.cfg.Notification("logins")
	if !sec.Bool("enabled", true) {
		return nil
	}

	return logwatch.New(tailer, clock, logwatch.Options{
		TrustedIPs:    sec.List("trusted_ip"),
		SuccessWindow: time.Duration(sec.Int("success_window", loginsSuccessWin)) * time.Second,
		FailureWindow: time.Duration(sec.Int("failure_window", loginsFailureWin)) * time.Second,
	})
}

func (e *env) thermalWatcher(svc *services) *thermal.Watcher {
	sec := e.cfg.Notification("thermal")
	if !sec.Bool("enabled", true) {
		return nil
	}

	return thermal.New(svc.system.Temperatures, svc.clock, thermal.Options{
		High:     float64(sec.Int("high", thermalHigh)),
		Normal:   float64(sec.Int("normal", thermalNormal)),
		Interval: time.Duration(sec.Int("interval", thermalInterval)) * time.Second,
		Remind:   time.Duration(sec.Int("remind", thermalRemind)) * time.Second,
	})
}

func (e *env) backupScheduler(svc *services, hostname func(context.Context) string) *backup.Scheduler {
	sec := e.cfg.Notification("backup")
	if !sec.Bool("enabled", true) || !svc.backuper.Available() {
		return nil
	}

	sched, err := backup.ParseSchedule(sec.Opt("day", backupDay), sec.Opt("time", backupTime))
	if err != nil {
		e.log.Warn("weekly backup disabled", "err", err)

		return nil
	}

	return backup.NewScheduler(svc.backup, state.New(e.persist()), svc.clock, sched, hostname)
}

// leaseFile reads dnsmasq's lease file path from UCI (default when unset).
func (e *env) leaseFile(ctx context.Context) string {
	pkg, err := e.uci.Get(ctx, "dhcp")
	if err != nil {
		return ""
	}

	for _, s := range pkg.OfType("dnsmasq") {
		if p := s.Opt("leasefile", ""); p != "" {
			return p
		}
	}

	return ""
}

func (e *env) persist() string {
	if e.fake {
		return filepath.Join(e.tmpDir(), "wrtgram-etc")
	}

	return persistDir
}

func (e *env) volatile() string {
	if e.fake {
		return filepath.Join(e.tmpDir(), "wrtgram")
	}

	return volatileDir
}

func listOr(l, def []string) []string {
	if len(l) > 0 {
		return l
	}

	return def
}

// builtinNames reports whether a custom command would shadow a built-in one
// (the custom module's own commands are not built-ins).
func builtinNames(reg *module.Registry) func(string) bool {
	return func(name string) bool {
		if name == module.HelpCommand || name == module.StartCommand {
			return true
		}

		return reg.Has(name) && reg.Owner(name) != custommod.Name
	}
}
