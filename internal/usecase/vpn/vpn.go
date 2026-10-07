// Package vpn reports tunnel state and switches the VPN on and off: through
// the failover controller when it runs, otherwise by bringing the netifd
// interfaces up or down.
package vpn

import (
	"context"
	"fmt"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// Failover is what the vpn usecase needs from the failover controller.
type Failover interface {
	State() entity.FailoverState
	SetManualOff(ctx context.Context, off bool) error
}

// Service is the vpn usecase.
type Service struct {
	tunnels  usecase.Tunnels
	ubus     usecase.Ubus
	failover Failover // nil when the failover module is off
}

// New creates the service.
func New(tunnels usecase.Tunnels, ubus usecase.Ubus, failover Failover) *Service {
	return &Service{tunnels: tunnels, ubus: ubus, failover: failover}
}

// Failover returns the controller or nil.
func (s *Service) Failover() Failover { return s.failover }

// List returns every configured tunnel with its live state.
func (s *Service) List(ctx context.Context) ([]entity.Tunnel, error) {
	refs, err := s.tunnels.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tunnels: %w", err)
	}

	out := make([]entity.Tunnel, 0, len(refs))

	for _, ref := range refs {
		t, err := s.tunnels.Status(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("tunnel %s: %w", ref.Name, err)
		}

		out = append(out, t)
	}

	return out, nil
}

// SetEnabled switches the VPN. With failover it flips the manual-off flag
// (routes change, tunnels stay up so the bot keeps working); without it the
// tunnel interfaces are brought down or up.
func (s *Service) SetEnabled(ctx context.Context, on bool) error {
	if s.failover != nil {
		return s.failover.SetManualOff(ctx, !on)
	}

	refs, err := s.tunnels.List(ctx)
	if err != nil {
		return fmt.Errorf("list tunnels: %w", err)
	}

	method := "down"
	if on {
		method = "up"
	}

	for _, ref := range refs {
		if err := s.ubus.Call(ctx, "network.interface."+ref.Name, method, nil, nil); err != nil {
			return fmt.Errorf("%s %s: %w", method, ref.Name, err)
		}
	}

	return nil
}
