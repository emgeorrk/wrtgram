package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/emgeorrk/wrtgram/internal/controller/telegram"
	"github.com/emgeorrk/wrtgram/internal/module"
	tgrepo "github.com/emgeorrk/wrtgram/internal/repo/telegram"
	"github.com/emgeorrk/wrtgram/internal/usecase/notify"
)

const (
	getMeRetryMin = 5 * time.Second
	getMeRetryMax = 2 * time.Minute
	notifierMin   = 5 * time.Second
	notifierMax   = time.Minute
	drainGrace    = 3 * time.Second
	backoffFactor = 2
)

// realClock implements usecase.Clock with the wall clock.
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

func runDaemon(ctx context.Context, version string) error {
	e, err := setup(ctx, true, true)
	if err != nil {
		return err
	}

	svc := e.buildServices()

	reg, err := e.buildRegistry(ctx, svc)
	if err != nil {
		return err
	}

	warns, err := e.cfg.Validate(builtinNames(reg))
	if err != nil {
		return fmt.Errorf("%w: %w", errConfig, err)
	}

	for _, w := range warns {
		e.log.Warn(string(w))
	}

	tg, err := tgrepo.New(e.cfg.Main.Token, e.cfg.Main.Proxy, e.log)
	if err != nil {
		return err
	}

	clock := realClock{}
	queue := notify.New(tg, clock, e.notifyChats(), e.log)

	ctl := telegram.New(tg, tg, reg, clock, e.log, telegram.Options{
		Allowed:     e.cfg.Allowed,
		NoAllowlist: len(e.cfg.Main.ChatIDs) == 0,
		Workers:     e.cfg.Main.Workers,
	})

	e.log.Info("wrtgram starting", "version", version, "modules", len(reg.Modules()))

	if err := waitForTelegram(ctx, tg, e); err != nil {
		return err
	}

	if err := ctl.SetMenu(ctx); err != nil {
		e.log.Warn("command menu not set", "err", err)
	}

	wg := startBackground(ctx, e, reg, queue)

	ctl.Run(ctx)
	wg.Wait()

	e.log.Info("wrtgram stopped")

	return nil
}

func (e *env) notifyChats() []int64 {
	if len(e.cfg.Main.NotifyChatIDs) > 0 {
		return e.cfg.Main.NotifyChatIDs
	}

	return e.cfg.Main.ChatIDs
}

// startBackground runs the notification queue and every notifier.
func startBackground(ctx context.Context, e *env, reg *module.Registry, queue *notify.Queue) *sync.WaitGroup {
	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		queue.Run(ctx, drainGrace)
	}()

	for _, n := range reg.Notifiers() {
		wg.Add(1)

		go func(n module.Notifier) {
			defer wg.Done()

			superviseNotifier(ctx, n, queue, e)
		}(n)
	}

	return &wg
}

// waitForTelegram retries getMe until it succeeds: right after boot the
// clock is often wrong and TLS fails until NTP has synced.
func waitForTelegram(ctx context.Context, tg *tgrepo.Client, e *env) error {
	wait := getMeRetryMin

	for {
		me, err := tg.Me(ctx)
		if err == nil {
			e.log.Info("connected to telegram", "bot", "@"+me.Username)

			return nil
		}

		if errors.Is(err, context.Canceled) {
			return err
		}

		e.log.Warn("telegram unreachable, retrying", "err", err, "in", wait)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}

		wait = min(wait*backoffFactor, getMeRetryMax)
	}
}

// superviseNotifier restarts a notifier with backoff when it fails.
func superviseNotifier(ctx context.Context, n module.Notifier, queue *notify.Queue, e *env) {
	wait := notifierMin

	for {
		start := time.Now()

		err := n.Run(ctx, queue.Notify)
		if ctx.Err() != nil {
			return
		}

		if err != nil {
			e.log.Error("notifier stopped", "notifier", n.Name(), "err", err)
		}

		if time.Since(start) > notifierMax {
			wait = notifierMin
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		wait = min(wait*backoffFactor, notifierMax)
	}
}
