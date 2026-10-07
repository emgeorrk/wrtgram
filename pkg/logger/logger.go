// Package logger builds the process logger: slog text output with secrets
// masked in every message and attribute. Under procd the output goes to
// syslog, which adds its own timestamp, so the daemon omits slog's.
package logger

import (
	"io"
	"log/slog"
	"strings"
)

const mask = "***"

// Options configure New.
type Options struct {
	Secrets  []string
	Level    slog.Level
	OmitTime bool
}

// New creates a logger. Any occurrence of a secret in a message or string
// attribute is replaced by "***".
func New(w io.Writer, opts Options) *slog.Logger {
	var pairs []string

	for _, s := range opts.Secrets {
		if s != "" {
			pairs = append(pairs, s, mask)
		}
	}

	r := strings.NewReplacer(pairs...)
	hopts := &slog.HandlerOptions{
		Level: opts.Level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if opts.OmitTime && a.Key == slog.TimeKey {
				return slog.Attr{}
			}

			if len(pairs) > 0 && a.Value.Kind() == slog.KindString {
				a.Value = slog.StringValue(r.Replace(a.Value.String()))
			}

			return a
		},
	}

	return slog.New(slog.NewTextHandler(w, hopts))
}
