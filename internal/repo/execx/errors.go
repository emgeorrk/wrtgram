package execx

import "errors"

var (
	// ErrExit is returned when the program exits with a non-zero status; the
	// wrapped message carries the program name and its stderr.
	ErrExit = errors.New("command failed")
	// ErrNotFound is returned when the program is not in PATH.
	ErrNotFound = errors.New("command not found")
	// ErrTimeout is returned when ctx expired before the program finished.
	ErrTimeout = errors.New("command timed out")
	// ErrNoFixture is returned by the fake runner for an unknown command.
	ErrNoFixture = errors.New("no fixture for command")
)
