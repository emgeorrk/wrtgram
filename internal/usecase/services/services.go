// Package services lists procd services and restarts the whitelisted ones.
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// ErrNotAllowed is returned for a restart of a service outside the whitelist.
var ErrNotAllowed = errors.New("service is not in the restart whitelist")

// Service is the services usecase.
type Service struct {
	ubus     usecase.Ubus
	run      usecase.Runner
	annotate Annotator
	allowed  map[string]bool
}

// New creates the usecase; allowed is the restart whitelist, annotate may be nil.
func New(ubus usecase.Ubus, run usecase.Runner, allowed []string, annotate Annotator) *Service {
	m := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		m[a] = true
	}

	return &Service{ubus: ubus, run: run, annotate: annotate, allowed: m}
}

// Allowed reports whether name may be restarted.
func (s *Service) Allowed(name string) bool { return s.allowed[name] }

// Whitelist returns the restartable service names.
func (s *Service) Whitelist() []string {
	out := make([]string, 0, len(s.allowed))
	for n := range s.allowed {
		out = append(out, n)
	}

	sort.Strings(out)

	return out
}

type listReply map[string]struct {
	Instances map[string]struct {
		Respawn  json.RawMessage `json:"respawn"`
		ExitCode int             `json:"exit_code"`
		Running  bool            `json:"running"`
	} `json:"instances"`
}

// Annotator returns extra status text for a service ("" for none).
type Annotator func(ctx context.Context, name string) string

// List returns every procd service with its instance counts, sorted by name.
func (s *Service) List(ctx context.Context) ([]entity.Service, error) {
	var reply listReply
	if err := s.ubus.Call(ctx, "service", "list", nil, &reply); err != nil {
		return nil, fmt.Errorf("service list: %w", err)
	}

	out := make([]entity.Service, 0, len(reply))

	for name, svc := range reply {
		e := entity.Service{Name: name, Instances: len(svc.Instances)}

		for _, inst := range svc.Instances {
			switch {
			case inst.Running:
				e.Running++
			case len(inst.Respawn) > 0 || inst.ExitCode != 0:
				// A daemon that should respawn, or a script that failed.
				e.Failed++
			}
		}

		if s.annotate != nil {
			e.Detail = s.annotate(ctx, name)
		}

		out = append(out, e)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

// Restart runs /etc/init.d/<name> restart for a whitelisted service.
func (s *Service) Restart(ctx context.Context, name string) error {
	if !s.allowed[name] {
		return fmt.Errorf("%w: %s", ErrNotAllowed, name)
	}

	if _, err := s.run.Run(ctx, "/etc/init.d/"+name, "restart"); err != nil {
		return fmt.Errorf("restart %s: %w", name, err)
	}

	return nil
}
