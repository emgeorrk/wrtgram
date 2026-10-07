// Package custom runs the shell commands users define in UCI.
package custom

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/emgeorrk/wrtgram/internal/repo/execx"
	"github.com/emgeorrk/wrtgram/internal/usecase"
)

// Executor runs user commands with a timeout.
type Executor struct {
	run usecase.Runner
}

// New creates the executor.
func New(run usecase.Runner) *Executor { return &Executor{run: run} }

// Exec runs command through /bin/sh and returns its combined output. A
// non-zero exit is reported inside the output rather than as an error, so
// the user sees what the command printed.
func (e *Executor) Exec(ctx context.Context, command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := e.run.Run(ctx, "/bin/sh", "-c", command)
	text := strings.TrimRight(string(out), "\n")

	switch {
	case err == nil:
		return text, nil
	case errors.Is(err, execx.ErrTimeout):
		return text + "\n[timed out after " + timeout.String() + "]", nil
	case errors.Is(err, execx.ErrExit):
		return text + "\n[" + err.Error() + "]", nil
	}

	return "", err
}
