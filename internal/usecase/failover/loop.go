package failover

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

const (
	// StateName is the document name in the volatile state store; other
	// scripts may read <dir>/failover.json.
	StateName = "failover"
	// restoreWindow bounds how old a saved state may be to survive a restart
	// without re-announcing the current situation.
	restoreWindow = 10 * time.Minute
)

// Deps are the ports the loop drives.
type Deps struct {
	Pinger usecase.Pinger
	Router usecase.Router
	WAN    usecase.WAN
	Store  usecase.StateStore // volatile: last state, re-read after a restart
	// Persist survives reboots and holds the manual switch. It lives outside
	// UCI on purpose: a `uci commit` would make procd restart the bot.
	Persist usecase.StateStore
	Clock   usecase.Clock
	Log     *slog.Logger
}

// ManualName is the document name of the manual switch in the persistent store.
const ManualName = "manual"

type manualDoc struct {
	Off bool `json:"off"`
}

// Loop is the running controller.
type Loop struct {
	deps      Deps
	trigger   chan chan struct{}
	state     entity.FailoverState
	rules     Rules
	mu        sync.Mutex
	ntpBroken bool
}

// NewLoop creates a controller and reads the persisted manual switch.
func NewLoop(rules Rules, deps Deps) (*Loop, error) {
	if len(rules.Tunnels) == 0 {
		return nil, errNoTunnels
	}

	var manual manualDoc

	manualOff := deps.Persist.Load(ManualName, &manual) == nil && manual.Off

	return &Loop{
		deps:    deps,
		trigger: make(chan chan struct{}, 1),
		rules:   rules,
		state:   Initial(rules, manualOff, deps.Clock.Now()),
	}, nil
}

// Rules returns the configuration.
func (l *Loop) Rules() Rules { return l.rules }

// State returns a copy of the current state.
func (l *Loop) State() entity.FailoverState {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.state
}

// SetManualOff flips the manual switch, persists it and applies the routes
// before returning, so the caller's reply reflects reality.
func (l *Loop) SetManualOff(ctx context.Context, off bool) error {
	if err := l.deps.Persist.Save(ManualName, manualDoc{Off: off}); err != nil {
		return fmt.Errorf("%w: %w", errPersist, err)
	}

	l.mu.Lock()
	l.state.ManualOff = off
	l.mu.Unlock()

	l.Tick(ctx)

	return nil
}

// Tick asks the loop for an immediate evaluation and waits for it.
func (l *Loop) Tick(ctx context.Context) {
	done := make(chan struct{})

	select {
	case l.trigger <- done:
	case <-ctx.Done():
		return
	default:
		return // a tick is already pending
	}

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Run executes the controller until ctx is done. onEvent receives every
// routing change worth telling the user about.
func (l *Loop) Run(ctx context.Context, onEvent func(entity.FailoverEvent)) error {
	l.restore()

	first := true

	for {
		l.tick(ctx, onEvent, first)

		first = false

		select {
		case <-ctx.Done():
			return nil
		case done := <-l.trigger:
			l.tick(ctx, onEvent, false)
			close(done)
		case <-l.deps.Clock.After(l.rules.Interval):
		}
	}
}

func (l *Loop) tick(ctx context.Context, onEvent func(entity.FailoverEvent), force bool) {
	if l.rules.NTPDirect && !l.ntpBroken {
		l.ntpDirect(ctx)
	}

	health := l.probe(ctx)
	now := l.deps.Clock.Now()

	l.mu.Lock()
	prev := l.state
	next, events := Step(prev, health, now, l.rules)
	l.state = next
	l.mu.Unlock()

	l.apply(ctx, prev, next, health, force)

	for _, ev := range events {
		l.deps.Log.Info("failover", "from", orDirect(ev.From), "to", orDirect(ev.To), "after", ev.Downtime.Round(time.Second))
		onEvent(ev)
	}

	if err := l.deps.Store.Save(StateName, next); err != nil {
		l.deps.Log.Warn("failover state not saved", "err", err)
	}
}

// probe pings every tunnel concurrently.
func (l *Loop) probe(ctx context.Context) map[string]bool {
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		health = make(map[string]bool, len(l.rules.Tunnels))
	)

	for _, dev := range l.rules.Tunnels {
		wg.Add(1)

		go func(dev string) {
			defer wg.Done()

			ok := false

			for _, target := range l.rules.Targets {
				if l.deps.Pinger.Ping(ctx, dev, target) {
					ok = true

					break
				}
			}

			mu.Lock()
			health[dev] = ok
			mu.Unlock()
		}(dev)
	}

	wg.Wait()

	return health
}

// apply reconciles the kernel with the decided state. The default route is
// checked every tick because ifup of a tunnel re-adds its own routes.
func (l *Loop) apply(ctx context.Context, prev, next entity.FailoverState, health map[string]bool, force bool) {
	cur, err := l.deps.Router.DefaultDev(ctx)
	if err != nil {
		l.deps.Log.Warn("failover: cannot read routes", "err", err)

		return
	}

	if cur != next.Dev || force {
		l.applyDefault(ctx, next.Dev, next.Dev == "" || health[next.Dev])
	}

	if len(l.rules.ServiceNets) > 0 && (next.ServiceDev != prev.ServiceDev || force) {
		l.applyService(ctx, next.ServiceDev)
	}
}

// applyDefault sets the default route. While a tunnel waits out its grace
// its device may be gone (ifdown), so a failure for an unhealthy tunnel is
// expected and only logged at debug level.
func (l *Loop) applyDefault(ctx context.Context, dev string, healthy bool) {
	var err error

	if dev == "" {
		err = l.deps.Router.ClearDefault(ctx)
	} else {
		err = l.deps.Router.SetDefault(ctx, dev)
	}

	switch {
	case err == nil:
	case healthy:
		l.deps.Log.Error("failover: route change failed", "dev", orDirect(dev), "err", err)
	default:
		l.deps.Log.Debug("failover: route not set, tunnel is down", "dev", dev, "err", err)
	}
}

func (l *Loop) applyService(ctx context.Context, dev string) {
	var err error

	if dev == "" {
		err = l.deps.Router.DeleteNets(ctx, l.rules.ServiceNets)
	} else {
		err = l.deps.Router.ReplaceNets(ctx, l.rules.ServiceNets, dev)
	}

	if err != nil {
		l.deps.Log.Error("failover: service routes failed", "err", err)
	}
}

func (l *Loop) ntpDirect(ctx context.Context) {
	if l.deps.WAN == nil {
		return
	}

	wan, err := l.deps.WAN.Status(ctx)
	if err != nil || wan.Device == "" {
		return
	}

	changed, err := l.deps.Router.EnsureNTPDirect(ctx, wan.Gateway, wan.Device)
	if err != nil {
		l.ntpBroken = true
		l.deps.Log.Warn("failover: ntp_direct disabled", "err", err)

		return
	}

	if changed {
		l.deps.Log.Info("failover: NTP pinned to the WAN gateway", "dev", wan.Device)
	}
}

// restore reloads a recent state so a bot restart stays silent.
func (l *Loop) restore() {
	var saved entity.FailoverState

	if err := l.deps.Store.Load(StateName, &saved); err != nil {
		l.deps.Log.Debug("failover state not restored", "err", err)

		return
	}

	if l.deps.Clock.Now().Sub(saved.CheckedAt) > restoreWindow {
		return
	}

	l.mu.Lock()
	saved.ManualOff = l.state.ManualOff // the persisted switch wins
	l.state = saved
	l.mu.Unlock()
}

func orDirect(dev string) string {
	if dev == "" {
		return "direct"
	}

	return dev
}
