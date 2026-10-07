// Package adblock reads the status file the adblock package maintains.
// adblock is a one-shot procd script, so procd alone cannot say whether
// blocking is active; the runtime JSON can.
package adblock

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"
)

// Runtime file paths (adblock 4.x, then the legacy 3.x location), relative
// to the filesystem root.
var runtimeFiles = []string{"var/run/adblock/adblock.runtime.json", "tmp/adb_runtime.json"} //nolint:gochecknoglobals // immutable lookup list

type runtime struct {
	Status  string `json:"adblock_status"`
	Blocked string `json:"blocked_domains"`
	LastRun string `json:"last_run"`
}

// Status returns a one-line summary such as "enabled, 218 400 domains",
// or "" when adblock has no runtime file.
func Status(fsys fs.FS) string {
	for _, p := range runtimeFiles {
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			continue
		}

		var rt runtime
		if err := json.Unmarshal(raw, &rt); err != nil || rt.Status == "" {
			continue
		}

		out := rt.Status
		if rt.Blocked != "" {
			out += ", " + strings.TrimSpace(rt.Blocked) + " domains"
		}

		return out
	}

	return ""
}

// Annotate adapts Status to the services usecase: only the adblock service
// gets the annotation.
func Annotate(fsys fs.FS) func(ctx context.Context, name string) string {
	return func(_ context.Context, name string) string {
		if name != "adblock" {
			return ""
		}

		return Status(fsys)
	}
}
