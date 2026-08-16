package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const noLocalShell = "a shell on this machine is only available in the desktop app"

type LocalShell interface {
	io.ReadWriter
	Resize(cols, rows uint16)
	Done() <-chan error
	Close()
}

type LocalShellOpener func(cols, rows uint16) (LocalShell, error)

func (s *Server) UseLocalShell(open LocalShellOpener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localShell = open
}

func (s *Server) localShellOpener() LocalShellOpener {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.localShell
}

func (s *Server) handleLocalShellSupport(w http.ResponseWriter, r *http.Request) {
	if s.localShellOpener() == nil {
		writeJSON(w, api.LocalShell{Reason: noLocalShell})
		return
	}
	writeJSON(w, api.LocalShell{Available: true})
}

func shellSize(r *http.Request) (uint16, uint16) {
	query := r.URL.Query()
	return dimension(query.Get("cols"), 80), dimension(query.Get("rows"), 24)
}

func dimension(raw string, fallback uint16) uint16 {
	value, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || value == 0 {
		return fallback
	}
	return uint16(value)
}

func (s *Server) handleLocalShell(w http.ResponseWriter, r *http.Request) {
	open := s.localShellOpener()
	if open == nil {
		writeError(w, http.StatusNotImplemented, noLocalShell)
		return
	}
	socket, err := accept(w, r)
	if err != nil {
		slog.Warn("a local shell upgrade was refused", "error", err)
		return
	}
	defer func() { _ = socket.CloseNow() }()
	s.trackExec(socket)
	defer s.forgetExec(socket)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	conn := &execConn{conn: socket, ctx: ctx}
	cols, rows := shellSize(r)
	shell, startErr := open(cols, rows)
	if startErr != nil {
		_ = conn.send(ctx, api.ExecChannelError, []byte(startErr.Error()))
		return
	}
	defer shell.Close()

	safe.Go("watching the local shell", func() {
		leftErr := <-shell.Done()
		_ = conn.send(ctx, api.ExecChannelError, endMessage(leftErr))
		cancel()
	})

	safe.Go("reading the local shell", func() {
		_, _ = io.Copy(conn, shell)
		cancel()
	})

	pumpLocalShell(ctx, socket, shell)
}

func pumpLocalShell(ctx context.Context, socket *websocket.Conn, shell LocalShell) {
	for {
		kind, data, err := socket.Read(ctx)
		if err != nil {
			return
		}
		if kind != websocket.MessageBinary {
			continue
		}
		if len(data) == 0 {
			continue
		}
		routeLocalShell(shell, data[0], data[1:])
	}
}

func routeLocalShell(shell LocalShell, channel byte, payload []byte) {
	switch channel {
	case api.ExecChannelStdin:
		_, _ = shell.Write(payload)
	case api.ExecChannelResize:
		resizeLocalShell(shell, payload)
	default:
		return
	}
}

func resizeLocalShell(shell LocalShell, payload []byte) {
	var size api.TerminalSize
	err := json.Unmarshal(payload, &size)
	if err != nil {
		return
	}
	shell.Resize(size.Cols, size.Rows)
}
