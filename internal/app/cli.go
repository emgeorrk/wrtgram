package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/module"
	systemmod "github.com/emgeorrk/wrtgram/internal/module/system"
)

// probe prints what the bot would see on this router: modules, board, link,
// sensors. It needs no token.
func probe(ctx context.Context) error {
	e, err := setup(ctx, false, false)
	if err != nil {
		return err
	}

	svc := e.buildServices()

	reg, err := e.buildRegistry(ctx, svc)
	if err != nil {
		return err
	}

	w := os.Stdout

	fmt.Fprintln(w, "Modules:")

	for _, m := range reg.Modules() {
		names := make([]string, 0, len(m.Commands()))
		for _, c := range m.Commands() {
			names = append(names, "/"+c.Name)
		}

		fmt.Fprintf(w, "  %-10s %v\n", m.Name(), names)
	}

	snap, err := svc.system.Snapshot(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "\nBoard:   %s (%s, %s)\nRelease: %s, kernel %s\n", snap.Board.Model, snap.Board.Target, snap.Board.Arch,
		snap.Board.Release, snap.Board.Kernel)
	fmt.Fprintf(w, "Uptime:  %s, load %.2f %.2f %.2f\n", module.FormatDuration(snap.Info.Uptime),
		snap.Info.Load[0], snap.Info.Load[1], snap.Info.Load[2])
	fmt.Fprintf(w, "Memory:  %s available of %s\n", module.FormatBytes(snap.Info.MemAvail), module.FormatBytes(snap.Info.MemTotal))
	fmt.Fprintf(w, "Flash:   %s free of %s (%s)\n", module.FormatBytes(snap.Info.FlashFree), module.FormatBytes(snap.Info.FlashTotal), e.overlay)
	printTemps(w, snap.Temps, snap.TempsErr)

	if snap.WANErr != nil {
		fmt.Fprintf(w, "WAN:     error: %v\n", snap.WANErr)
	} else {
		fmt.Fprintf(w, "WAN:     %s via %s up=%v addr=%v gw=%s\n", snap.WAN.Interface, snap.WAN.Device, snap.WAN.Up, snap.WAN.IPv4, snap.WAN.Gateway)
	}

	return nil
}

func printTemps(w io.Writer, temps []entity.Temperature, err error) {
	if err != nil {
		fmt.Fprintf(w, "Sensors: none (%v)\n", err)

		return
	}

	fmt.Fprint(w, "Sensors:")

	for _, t := range temps {
		fmt.Fprintf(w, " %s=%.1f°C", systemmod.SensorName(t), t.Celsius)
	}

	fmt.Fprintln(w)
}

// checkConfig validates /etc/config/wrtgram and prints the problems.
func checkConfig(ctx context.Context) error {
	e, err := setup(ctx, true, false)
	if err != nil {
		return err
	}

	svc := e.buildServices()

	reg, err := e.buildRegistry(ctx, svc)
	if err != nil {
		return err
	}

	warns, err := e.cfg.Validate(builtinNames(reg))
	for _, w := range warns {
		fmt.Fprintln(os.Stdout, "warning:", w)
	}

	if err != nil {
		return fmt.Errorf("%w: %w", errConfig, err)
	}

	fmt.Fprintf(os.Stdout, "config ok: %d chat(s), %d module(s) active, %d custom command(s)\n",
		len(e.cfg.Main.ChatIDs), len(reg.Modules()), len(e.cfg.Commands))

	return nil
}
