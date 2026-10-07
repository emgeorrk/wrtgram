package tgtext_test

import (
	"strings"
	"testing"

	"github.com/emgeorrk/wrtgram/pkg/tgtext"
)

func TestEsc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "router", want: "router"},
		{name: "tags", in: "<script>&", want: "&lt;script&gt;&amp;"},
		{name: "quotes kept", in: `a "b" 'c'`, want: `a "b" 'c'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tgtext.Esc(tt.in); got != tt.want {
				t.Fatalf("Esc(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplit(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("line of text\n", 20) // 260 chars

	tests := []struct {
		name       string
		in         string
		limit      int
		wantChunks int
		wantPre    bool
	}{
		{name: "fits", in: "hello", limit: 10, wantChunks: 1},
		{name: "lines", in: long, limit: 100, wantChunks: 3},
		{name: "pre reopened", in: "<pre>" + long + "</pre>", limit: 100, wantChunks: 4, wantPre: true},
		{name: "single long line", in: strings.Repeat("x", 250), limit: 100, wantChunks: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tgtext.Split(tt.in, tt.limit)
			if len(got) != tt.wantChunks {
				t.Fatalf("Split() produced %d chunks, want %d: %q", len(got), tt.wantChunks, got)
			}

			for i, c := range got {
				if n := len([]rune(c)); n > tt.limit {
					t.Errorf("chunk %d has %d chars > limit %d", i, n, tt.limit)
				}

				if tt.wantPre {
					if !strings.HasPrefix(c, "<pre>") || !strings.HasSuffix(c, "</pre>") {
						t.Errorf("chunk %d is not a closed <pre> block: %q", i, c)
					}
				}
			}

			joined := strings.Join(got, "")
			if !tt.wantPre && joined != tt.in {
				t.Errorf("chunks do not reassemble the input")
			}
		})
	}
}
