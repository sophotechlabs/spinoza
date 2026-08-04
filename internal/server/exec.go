package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
)

const execWriteTimeout = 10 * time.Second

var execStdinTimeout = 10 * time.Second

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
	support, err := s.manager().ExecSupport(r.Context(), req)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, support)
}

func (s *Server) handleDebugSupport(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	namespace := query.Get("namespace")
	if namespace == "" {
		writeError(w, http.StatusBadRequest, "namespace is required")
		return
	}
	writeJSON(w, s.manager().DebugSupport(r.Context(), namespace, query.Get("pod")))
}

func (s *Server) handleDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	query := r.URL.Query()
	req := debugcontainer.Request{
		Namespace: query.Get("namespace"),
		Pod:       query.Get("pod"),
		Container: query.Get("container"),
		Profile:   query.Get("profile"),
	}
	if req.Namespace == "" || req.Pod == "" {
		writeError(w, http.StatusBadRequest, "namespace and pod are required")
		return
	}
	session, err := s.manager().StartDebug(r.Context(), req)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, session)
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	req := execRequest(r)
	if req.Namespace == "" || req.Pod == "" {
		writeError(w, http.StatusBadRequest, "namespace and pod are required")
		return
	}
	socket, err := accept(w, r)
	if err != nil {
		return
	}
	defer func() { _ = socket.CloseNow() }()
	s.trackExec(socket)
	defer s.forgetExec(socket)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	conn := &execConn{conn: socket, ctx: ctx}
	session, startErr := s.manager().StartExec(ctx, req, conn)
	if startErr != nil {
		_ = conn.send(ctx, api.ExecChannelError, []byte(startErr.Error()))
		return
	}
	defer session.Close()

	go func() {
		streamErr := <-session.Done()
		_ = conn.send(ctx, api.ExecChannelError, endMessage(streamErr))
		cancel()
	}()

	conn.pump(ctx, socket, session)
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

func (e *execConn) send(ctx context.Context, channel byte, payload []byte) error {
	frame := make([]byte, 0, len(payload)+1)
	frame = append(frame, channel)
	frame = append(frame, payload...)

	e.mu.Lock()
	defer e.mu.Unlock()
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), execWriteTimeout)
	defer cancel()
	return e.conn.Write(writeCtx, websocket.MessageBinary, frame)
}

func (e *execConn) Write(p []byte) (int, error) {
	err := e.send(e.ctx, api.ExecChannelStdout, p)
	if err != nil {
		return 0, fmt.Errorf("the terminal stopped reading: %w", err)
	}
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
		if !route(session, data[0], data[1:]) {
			return
		}
	}
}

func route(session *exec.Session, channel byte, payload []byte) bool {
	switch channel {
	case api.ExecChannelStdin:
		return writeStdin(session, payload)
	case api.ExecChannelResize:
		resize(session, payload)
		return true
	default:
		return true
	}
}

func writeStdin(session *exec.Session, payload []byte) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.Write(payload)
	}()
	timer := time.NewTimer(execStdinTimeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		session.Close()
		<-done
		return false
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
