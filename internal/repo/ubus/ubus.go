// Package ubus is the adapter for the OpenWrt message bus, driven through the
// `ubus` command line tool so no C bindings are needed.
package ubus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/usecase"
)

var (
	errCall   = errors.New("ubus call failed")
	errDecode = errors.New("ubus reply is not valid JSON")
)

// Client implements usecase.Ubus.
type Client struct {
	run usecase.Runner
}

// New creates a Client on top of a Runner.
func New(run usecase.Runner) *Client { return &Client{run: run} }

// Call invokes object.method. args is marshaled to JSON (nil → "{}"); the
// reply is decoded into out when out is not nil.
func (c *Client) Call(ctx context.Context, object, method string, args, out any) error {
	payload := "{}"

	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return fmt.Errorf("%w: %s.%s: %w", errCall, object, method, err)
		}

		payload = string(b)
	}

	raw, err := c.run.Run(ctx, "ubus", "call", object, method, payload)
	if err != nil {
		return fmt.Errorf("%w: %s.%s: %w", errCall, object, method, err)
	}

	if out == nil || len(raw) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: %s.%s: %w", errDecode, object, method, err)
	}

	return nil
}

// List returns the object names matching pattern (e.g. "hostapd.*").
func (c *Client) List(ctx context.Context, pattern string) ([]string, error) {
	raw, err := c.run.Run(ctx, "ubus", "list", pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: list %s: %w", errCall, pattern, err)
	}

	var names []string

	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}

	return names, nil
}
