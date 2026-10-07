// Package execx runs external programs for the adapters. The real Runner
// wraps os/exec; Fake replays recorded outputs for tests and the host dev mode.
package execx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

const (
	maxStdout   = 1 << 20 // 1 MiB
	maxStderr   = 64 << 10
	maxLine     = 64 << 10
	streamCap   = 64
	waitDelay   = 2 * time.Second
	stderrLines = 3
)

// Runner executes programs found in PATH.
type Runner struct {
	env []string
}

// New creates a Runner; extra env entries ("K=V") are appended to the process env.
func New(env ...string) *Runner { return &Runner{env: env} }

// Run executes name with args and returns its stdout (capped at 1 MiB).
func (r *Runner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return r.RunInput(ctx, nil, name, args...)
}

// RunInput is Run with stdin.
func (r *Runner) RunInput(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := r.command(ctx, name, args...)
	cmd.Stdin = stdin

	stdout := &limitedBuffer{max: maxStdout}
	stderr := &limitedBuffer{max: maxStderr}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		return stdout.Bytes(), wrapErr(ctx, name, err, stderr.Bytes())
	}

	return stdout.Bytes(), nil
}

// Stream starts the program and returns its stdout line by line. The channel
// closes when the program exits or ctx is done; the process is killed on
// cancellation.
func (r *Runner) Stream(ctx context.Context, name string, args ...string) (<-chan string, error) {
	cmd := r.command(ctx, name, args...)

	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrExit, name, err)
	}

	if err := cmd.Start(); err != nil {
		return nil, wrapErr(ctx, name, err, nil)
	}

	lines := make(chan string, streamCap)

	go func() {
		defer close(lines)
		defer cmd.Wait() //nolint:errcheck // the exit status is irrelevant once the stream ended

		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxLine)

		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	return lines, nil
}

// LookPath reports whether name is an executable in PATH.
func (r *Runner) LookPath(name string) bool {
	_, err := exec.LookPath(name)

	return err == nil
}

func (r *Runner) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = waitDelay

	if len(r.env) > 0 {
		cmd.Env = append(cmd.Environ(), r.env...)
	}

	return cmd
}

func wrapErr(ctx context.Context, name string, err error, stderr []byte) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %s", ErrTimeout, name)
	}

	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = err.Error()
	}

	return fmt.Errorf("%w: %s: %s", ErrExit, name, lastLines(msg, stderrLines))
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, " | ")
}

// limitedBuffer keeps the first max bytes and silently drops the rest.
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room < len(p) {
		if room > 0 {
			b.buf.Write(p[:room])
		}

		return len(p), nil
	}

	return b.buf.Write(p)
}

func (b *limitedBuffer) Bytes() []byte { return b.buf.Bytes() }
