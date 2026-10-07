package failover

import "errors"

var (
	errNoTunnels = errors.New("failover: no tunnels configured")
	errPersist   = errors.New("failover: cannot persist manual_off")
)
