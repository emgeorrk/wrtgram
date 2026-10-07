package execx

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Fake replays canned outputs. A command is matched by the longest registered
// argv prefix ("ubus call system board" matches "ubus call system board {}").
type Fake struct {
	outputs map[string][]byte
	errs    map[string]error
	streams map[string][]string
	paths   map[string]bool
	calls   []string
	mu      sync.Mutex
}

// NewFake creates an empty Fake.
func NewFake() *Fake {
	return &Fake{
		outputs: make(map[string][]byte),
		errs:    make(map[string]error),
		streams: make(map[string][]string),
		paths:   make(map[string]bool),
	}
}

// On registers stdout for an argv prefix.
func (f *Fake) On(argv string, out []byte) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.outputs[argv] = out

	return f
}

// OnString is On with a string.
func (f *Fake) OnString(argv, out string) *Fake { return f.On(argv, []byte(out)) }

// OnError makes an argv prefix fail with err.
func (f *Fake) OnError(argv string, err error) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.errs[argv] = err

	return f
}

// OnStream registers the lines a streamed command produces.
func (f *Fake) OnStream(argv string, lines []string) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.streams[argv] = lines

	return f
}

// Path declares whether a program exists for LookPath.
func (f *Fake) Path(name string, exists bool) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.paths[name] = exists

	return f
}

// Calls returns every executed command line, in order.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.calls...)
}

// Run implements usecase.Runner.
func (f *Fake) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f.RunInput(ctx, nil, name, args...)
}

// RunInput implements usecase.Runner; stdin is ignored.
func (f *Fake) RunInput(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
	line := argvLine(name, args)

	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, line)

	if key, ok := longestPrefix(f.errs, line); ok {
		return nil, f.errs[key]
	}

	if key, ok := longestPrefix(f.outputs, line); ok {
		return f.outputs[key], nil
	}

	return nil, fmt.Errorf("%w: %s", ErrNoFixture, line)
}

// Stream implements usecase.Runner.
func (f *Fake) Stream(ctx context.Context, name string, args ...string) (<-chan string, error) {
	line := argvLine(name, args)

	f.mu.Lock()
	f.calls = append(f.calls, line)
	key, ok := longestPrefix(f.streams, line)
	lines := f.streams[key]
	f.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoFixture, line)
	}

	ch := make(chan string, len(lines))

	go func() {
		defer close(ch)

		for _, l := range lines {
			select {
			case ch <- l:
			case <-ctx.Done():
				return
			}
		}

		<-ctx.Done()
	}()

	return ch, nil
}

// LookPath implements usecase.Runner. Programs not declared with Path fall
// back to the host PATH, so a fixture set stays small.
func (f *Fake) LookPath(name string) bool {
	f.mu.Lock()
	exists, declared := f.paths[name]
	f.mu.Unlock()

	if declared {
		return exists
	}

	_, err := exec.LookPath(name)

	return err == nil
}

func argvLine(name string, args []string) string {
	if len(args) == 0 {
		return name
	}

	return name + " " + strings.Join(args, " ")
}

func longestPrefix[V any](m map[string]V, line string) (string, bool) {
	best, found := "", false

	for k := range m {
		if (line == k || strings.HasPrefix(line, k+" ")) && len(k) > len(best) {
			best, found = k, true
		}
	}

	return best, found
}
