// Package ipc is the local control socket: the CLI (`wrtgram notify`,
// `wrtgram send-file`, `wrtgram event dhcp`) talks to the running daemon
// over a unix socket with one JSON request per connection.
package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/emgeorrk/wrtgram/internal/entity"
)

// DefaultSocket is the daemon's socket path on the router.
const DefaultSocket = "/var/run/wrtgram.sock"

const (
	socketMode  = 0o600
	connTimeout = 10 * time.Second
	maxRequest  = 64 << 10
)

// Operations.
const (
	OpNotify = "notify"
	OpFile   = "file"
	OpEvent  = "event"
)

var (
	errBadRequest = errors.New("ipc: bad request")
	errUnknownOp  = errors.New("ipc: unknown op")
)

// Request is one CLI call.
type Request struct {
	Event   *entity.DHCPEvent `json:"event,omitempty"`
	Op      string            `json:"op"`
	Text    string            `json:"text,omitempty"`
	Path    string            `json:"path,omitempty"`
	Caption string            `json:"caption,omitempty"`
}

// Response is the daemon's answer.
type Response struct {
	Error string `json:"error,omitempty"`
	OK    bool   `json:"ok"`
}

// Handler serves the operations.
type Handler interface {
	Notify(ctx context.Context, text string)
	SendFile(ctx context.Context, path, caption string) error
	DHCPEvent(ctx context.Context, ev entity.DHCPEvent)
}

// Server listens on the unix socket.
type Server struct {
	handler Handler
	log     *slog.Logger
	path    string
}

// NewServer creates a server for path.
func NewServer(path string, handler Handler, log *slog.Logger) *Server {
	return &Server{handler: handler, log: log, path: path}
}

// Run serves until ctx is done; the socket file is removed on exit.
func (s *Server) Run(ctx context.Context) error {
	_ = os.Remove(s.path)

	ln, err := (&net.ListenConfig{}).Listen(ctx, "unix", s.path)
	if err != nil {
		return fmt.Errorf("ipc listen: %w", err)
	}

	if err := os.Chmod(s.path, socketMode); err != nil {
		s.log.Warn("ipc: chmod socket", "err", err)
	}

	go func() {
		<-ctx.Done()
		ln.Close()

		_ = os.Remove(s.path)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("ipc accept: %w", err)
		}

		go s.serve(ctx, conn)
	}
}

func (s *Server) serve(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(connTimeout)); err != nil {
		s.log.Debug("ipc: deadline", "err", err)
	}

	var req Request

	dec := json.NewDecoder(bufio.NewReaderSize(conn, maxRequest))
	if err := dec.Decode(&req); err != nil {
		s.reply(conn, fmt.Errorf("%w: %w", errBadRequest, err))

		return
	}

	s.reply(conn, s.dispatch(ctx, req))
}

func (s *Server) dispatch(ctx context.Context, req Request) error {
	switch req.Op {
	case OpNotify:
		if req.Text == "" {
			return fmt.Errorf("%w: empty text", errBadRequest)
		}

		s.handler.Notify(ctx, req.Text)

		return nil
	case OpFile:
		return s.handler.SendFile(ctx, req.Path, req.Caption)
	case OpEvent:
		if req.Event == nil {
			return fmt.Errorf("%w: missing event", errBadRequest)
		}

		s.handler.DHCPEvent(ctx, *req.Event)

		return nil
	}

	return fmt.Errorf("%w: %q", errUnknownOp, req.Op)
}

func (s *Server) reply(conn net.Conn, err error) {
	resp := Response{OK: err == nil}
	if err != nil {
		resp.Error = err.Error()
	}

	if encErr := json.NewEncoder(conn).Encode(resp); encErr != nil {
		s.log.Debug("ipc: reply", "err", encErr)
	}
}

// Call sends one request to the daemon and returns its error, if any.
// ErrNoDaemon is returned when the socket does not exist.
func Call(ctx context.Context, path string, req Request) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%w: %s", ErrNoDaemon, path)
	}

	var d net.Dialer

	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoDaemon, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(connTimeout)); err != nil {
		return fmt.Errorf("ipc: deadline: %w", err)
	}

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("ipc: send: %w", err)
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("ipc: no reply: %w", err)
	}

	if !resp.OK {
		return fmt.Errorf("%w: %s", ErrRejected, resp.Error)
	}

	return nil
}

// Client-side errors.
var (
	ErrNoDaemon = errors.New("wrtgram daemon is not running")
	ErrRejected = errors.New("daemon rejected the request")
)
