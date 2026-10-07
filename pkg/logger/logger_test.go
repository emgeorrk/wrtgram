package logger_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/emgeorrk/wrtgram/pkg/logger"
)

func TestRedaction(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	const token = "123:SECRETSECRET"

	log := logger.New(&buf, logger.Options{Level: slog.LevelDebug, Secrets: []string{token, ""}, OmitTime: true})
	log.Info("request failed: https://api.telegram.org/bot"+token+"/getMe", "url", "x/bot"+token)
	log.Debug("debug visible")

	out := buf.String()
	if strings.Contains(out, token) {
		t.Fatalf("token leaked into log: %s", out)
	}

	if strings.Count(out, "***") != 2 {
		t.Errorf("expected two masked occurrences: %s", out)
	}

	if !strings.Contains(out, "debug visible") {
		t.Errorf("debug level not honoured: %s", out)
	}

	if strings.Contains(out, "time=") {
		t.Errorf("time must be omitted: %s", out)
	}
}
