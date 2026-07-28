package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/exec"
)

const execWriteTimeout = 10 * time.Second

func execRequest(r *http.Request) exec.Request {
	q := r.URL.Query()
	return exec.Request{
		Namespace: q.Get("namespace"),
		Pod:       q.Get("pod"),
		Container: q.Get("container"),
	}
}

func (s *Server) handleExecSupport(w http.ResponseWriter, r *http.Request) {
	req := execRequest(r)
	if req.Namespace == "" || req.Pod == "" {
		writeError(w, http.StatusBadRequest, "namespace and pod are required")
		return
	}
	support, err := s.mgr.ExecSupport(r.Context(), req)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, support)
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	req := execRequest(r)
	if req.Namespace == "" || req.Pod == "" {
		writeError(w, http.StatusBadRequest, "namespace and pod are required")
		return
	}
	c, err := accept(w, r)
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	conn := &execConn{conn: c, ctx: ctx}
	session, startErr := s.mgr.StartExec(ctx, req, conn)
	if startErr != nil {
		conn.send(api.ExecChannelError, []byte(startErr.Error()))
		return
	}
	defer session.Close()

	go func() {
		streamErr := <-session.Done()
		conn.send(api.ExecChannelError, endMessage(streamErr))
		cancel()
	}()

	conn.pump(ctx, c, session)
}

func endMessage(err error) []byte {
	if err == nil {
		return nil
	}
	return []byte(err.Error())
}

type execConn struct {
	conn *websocket.Conn
	ctx  context.Context
	mu   sync.Mutex
}

func (e *execConn) send(channel byte, payload []byte) {
	frame := make([]byte, 0, len(payload)+1)
	frame = append(frame, channel)
	frame = append(frame, payload...)

	e.mu.Lock()
	defer e.mu.Unlock()
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(e.ctx), execWriteTimeout)
	defer cancel()
	_ = e.conn.Write(writeCtx, websocket.MessageBinary, frame)
}

func (e *execConn) Write(p []byte) (int, error) {
	e.send(api.ExecChannelStdout, p)
	return len(p), nil
}

func (e *execConn) pump(ctx context.Context, c *websocket.Conn, session *exec.Session) {
	for {
		kind, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		if kind != websocket.MessageBinary {
			continue
		}
		if len(data) == 0 {
			continue
		}
		route(session, data[0], data[1:])
	}
}

func route(session *exec.Session, channel byte, payload []byte) {
	switch channel {
	case api.ExecChannelStdin:
		_, _ = session.Write(payload)
	case api.ExecChannelResize:
		resize(session, payload)
	default:
	}
}

func resize(session *exec.Session, payload []byte) {
	var size exec.Size
	err := json.Unmarshal(payload, &size)
	if err != nil {
		return
	}
	session.Resize(size)
}
