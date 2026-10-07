// Package uci reads and writes UCI configuration through rpcd's `uci` ubus
// object. The reply carries typed values (lists as arrays), and the package
// name is the only thing that ever appears in argv.
package uci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/emgeorrk/wrtgram/internal/entity"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

var (
	errGet    = errors.New("uci get failed")
	errSet    = errors.New("uci set failed")
	errCommit = errors.New("uci commit failed")
	errFile   = errors.New("uci json file")
)

const keyConfig = "config"

// Client implements usecase.UCI and config.Source.
type Client struct {
	ubus usecase.Ubus
}

// New creates a Client.
func New(ubus usecase.Ubus) *Client { return &Client{ubus: ubus} }

type getReply struct {
	Values map[string]map[string]json.RawMessage `json:"values"`
}

// Get returns the whole package.
func (c *Client) Get(ctx context.Context, pkg string) (entity.UCIPackage, error) {
	var reply getReply

	err := c.ubus.Call(ctx, "uci", "get", map[string]string{keyConfig: pkg}, &reply)
	if err != nil {
		return entity.UCIPackage{}, fmt.Errorf("%w: %s: %w", errGet, pkg, err)
	}

	return Decode(pkg, reply.Values)
}

// Set changes options of a section (uncommitted). Values are strings or
// []string for lists.
func (c *Client) Set(ctx context.Context, pkg, section string, values map[string]any) error {
	args := map[string]any{keyConfig: pkg, "section": section, "values": values}

	if err := c.ubus.Call(ctx, "uci", "set", args, nil); err != nil {
		return fmt.Errorf("%w: %s.%s: %w", errSet, pkg, section, err)
	}

	return nil
}

// AddSection creates a named section (uncommitted); an existing one is kept.
func (c *Client) AddSection(ctx context.Context, pkg, typ, name string) error {
	args := map[string]any{keyConfig: pkg, "type": typ, "name": name}

	if err := c.ubus.Call(ctx, "uci", "add", args, nil); err != nil {
		return fmt.Errorf("%w: add %s.%s: %w", errSet, pkg, name, err)
	}

	return nil
}

// Commit writes pending changes of a package to flash.
func (c *Client) Commit(ctx context.Context, pkg string) error {
	if err := c.ubus.Call(ctx, "uci", "commit", map[string]string{keyConfig: pkg}, nil); err != nil {
		return fmt.Errorf("%w: %s: %w", errCommit, pkg, err)
	}

	return nil
}

// Decode converts the "values" object of a `uci get` reply into a package.
func Decode(pkg string, values map[string]map[string]json.RawMessage) (entity.UCIPackage, error) {
	out := entity.UCIPackage{Name: pkg, Sections: make(map[string]entity.UCISection, len(values))}

	for name, raw := range values {
		sec, err := decodeSection(name, raw)
		if err != nil {
			return entity.UCIPackage{}, fmt.Errorf("%w: %s.%s: %w", errGet, pkg, name, err)
		}

		out.Sections[name] = sec
	}

	return out, nil
}

func decodeSection(name string, raw map[string]json.RawMessage) (entity.UCISection, error) {
	sec := entity.UCISection{
		Name:    name,
		Options: make(map[string]string),
		Lists:   make(map[string][]string),
	}

	for key, val := range raw {
		switch key {
		case ".type":
			if err := json.Unmarshal(val, &sec.Type); err != nil {
				return sec, err
			}
		case ".index":
			if err := json.Unmarshal(val, &sec.Index); err != nil {
				return sec, err
			}
		case ".name", ".anonymous":
		default:
			if err := decodeOption(&sec, key, val); err != nil {
				return sec, err
			}
		}
	}

	return sec, nil
}

func decodeOption(sec *entity.UCISection, key string, val json.RawMessage) error {
	if strings.HasPrefix(string(val), "[") {
		var list []string
		if err := json.Unmarshal(val, &list); err != nil {
			return err
		}

		sec.Lists[key] = list

		return nil
	}

	var s string
	if err := json.Unmarshal(val, &s); err != nil {
		return err
	}

	sec.Options[key] = s

	return nil
}

// File is a config.Source reading a `uci get` reply ({"values": {...}}) from
// a JSON file — the development path on a workstation.
type File string

// Get implements config.Source.
func (f File) Get(_ context.Context, pkg string) (entity.UCIPackage, error) {
	raw, err := os.ReadFile(string(f))
	if err != nil {
		return entity.UCIPackage{}, fmt.Errorf("%w: %w", errFile, err)
	}

	var reply getReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return entity.UCIPackage{}, fmt.Errorf("%w: %s: %w", errFile, f, err)
	}

	return Decode(pkg, reply.Values)
}
