package logread_test

import (
	"testing"
	"time"

	"github.com/emgeorrk/wrtgram/internal/repo/logread"
)

func TestParse(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("MSK", 3*3600)

	line, err := logread.Parse("Wed Oct  7 14:02:40 2026 authpriv.notice dropbear[21949]: Pubkey auth succeeded for 'root' from 192.168.1.222:50661", loc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if line.At != time.Date(2026, 10, 7, 14, 2, 40, 0, loc) || line.Facility != "authpriv.notice" || line.Tag != "dropbear" ||
		line.Message != "Pubkey auth succeeded for 'root' from 192.168.1.222:50661" {
		t.Errorf("Parse() = %+v", line)
	}

	if _, err := logread.Parse("garbage", loc); err == nil {
		t.Error("garbage must fail")
	}

	noTag, err := logread.Parse("Wed Oct  7 14:02:40 2026 kern.info message without tag", loc)
	if err != nil || noTag.Tag != "" || noTag.Message != "message without tag" {
		t.Errorf("no tag line = %+v, %v", noTag, err)
	}
}
