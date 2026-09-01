package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/nodeshell"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const execWriteTimeout = 10 * time.Second

const removeTimeout = 15 * time.Second

var debugTimeout = 3 * time.Minute

var nodeShellDrain = 20 * time.Second

var nodeShells shellTally

var execStdinTimeout = 10 * time.Second

func execRequest(r *http.Request) exec.Request {
	q := r.URL.Query()
	return exec.Request{
		Namespace: q.Get("namespace"),
		Pod:       q.Get("pod"),
		Container: q.Get("container"),
	}
}

func containerDetail(container string) string {
	if container == "" {
		return ""
	}
	return "into " + container
}

func debugDetail(req debugcontainer.Request) string {
	if req.Profile == "" {
		return containerDetail(req.Container)
	}
	return "with the " + req.Profile + " profile"
}

func (s *Server) handleExecSupport(w http.ResponseWriter, r *http.Request) {
	req := execRequest(r)
	if req.Namespace == "" || req.Pod == "" {
		writeError(w, http.StatusBadRequest, "namespace and pod are required")
		return
	}
	support, err := s.managerFor(r).ExecSupport(r.Context(), req)
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
	writeJSON(w, s.managerFor(r).DebugSupport(r.Context(), namespace, query.Get("pod")))
}

func (s *Server) handleDebug(w http.ResponseWriter, r *http.Request) {
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
	writer, kept, stop, ok := s.writingWithin(w, r, req.Pod, debugTimeout)
	if !ok {
		return
	}
	defer stop()
	//nolint:contextcheck // writing detaches r.Context so an abandoned request still finishes the write
	session, err := writer.StartDebug(kept, req)
	s.record(r, change{
		verb:   verbDebug,
		ref:    podRef(req.Namespace, req.Pod),
		kind:   kindPod,
		detail: debugDetail(req),
		err:    err,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, session)
}

func (s *Server) handleNodeShellSupport(w http.ResponseWriter, r *http.Request) {
	node := r.URL.Query().Get("node")
	if node == "" {
		writeError(w, http.StatusBadRequest, "node is required")
		return
	}
	writeJSON(w, s.managerFor(r).NodeShellSupport(r.Context(), node))
}

func (s *Server) handleNodeShell(w http.ResponseWriter, r *http.Request) {
	node := r.URL.Query().Get("node")
	if node == "" {
		writeError(w, http.StatusBadRequest, "node is required")
		return
	}
	release, allowed := s.claimLiveConnection(r)
	if !allowed {
		writeError(w, http.StatusTooManyRequests, "too many live connections are already open")
		return
	}
	defer release()
	writer, ok := s.writingSocket(w, r, node)
	if !ok {
		return
	}
	socket, err := accept(w, r)
	if err != nil {
		slog.Warn("a node shell upgrade was refused", "node", node, "error", err)
		return
	}
	defer func() { _ = socket.CloseNow() }()
	s.trackExec(socket, s.clusterKey(r))
	defer s.forgetExec(socket)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	s.revalidateLive(ctx, cancel, r, "revalidating the node shell on "+node)

	backend := s.managerFor(r)
	conn := &execConn{conn: socket, ctx: ctx}
	shell, startErr := writer.StartNodeShell(ctx, node)
	s.record(r, change{verb: verbNodeShell, ref: nodeRef(node), kind: kindNode, err: startErr})
	if startErr != nil {
		_ = conn.send(ctx, api.ExecChannelError, []byte(startErr.Error()))
		return
	}
	nodeShells.start()
	defer nodeShells.done()
	defer func() {
		gone, stop := context.WithTimeout(context.WithoutCancel(ctx), removeTimeout)
		defer stop()
		writer.RemoveNodeShell(gone, shell.Pod)
	}()

	req := exec.Request{
		Namespace: shell.Namespace,
		Pod:       shell.Pod,
		Container: shell.Container,
		Command:   nodeshell.Enter,
	}
	session, sessionErr := backend.StartExec(ctx, req, conn)
	if sessionErr != nil {
		_ = conn.send(ctx, api.ExecChannelError, []byte(sessionErr.Error()))
		return
	}
	defer session.Close()

	safe.Go("watching the node shell on "+node, func() {
		streamErr := <-session.Done()
		_ = conn.send(ctx, api.ExecChannelError, endMessage(streamErr))
		cancel()
	})

	conn.pump(ctx, socket, session)
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	req := execRequest(r)
	if req.Namespace == "" || req.Pod == "" {
		writeError(w, http.StatusBadRequest, "namespace and pod are required")
		return
	}
	release, allowed := s.claimLiveConnection(r)
	if !allowed {
		writeError(w, http.StatusTooManyRequests, "too many live connections are already open")
		return
	}
	defer release()
	socket, err := accept(w, r)
	if err != nil {
		slog.Warn("a terminal upgrade was refused", "namespace", req.Namespace, "pod", req.Pod, "error", err)
		return
	}
	defer func() { _ = socket.CloseNow() }()
	s.trackExec(socket, s.clusterKey(r))
	defer s.forgetExec(socket)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	s.revalidateLive(ctx, cancel, r, "revalidating the terminal in "+req.Namespace+"/"+req.Pod)

	backend := s.managerFor(r)
	conn := &execConn{conn: socket, ctx: ctx}
	session, startErr := backend.StartExec(ctx, req, conn)
	s.record(r, change{
		verb:   verbExec,
		ref:    podRef(req.Namespace, req.Pod),
		kind:   kindPod,
		detail: containerDetail(req.Container),
		err:    startErr,
	})
	if startErr != nil {
		_ = conn.send(ctx, api.ExecChannelError, []byte(startErr.Error()))
		return
	}
	defer session.Close()

	safe.Go("watching the terminal in "+req.Namespace+"/"+req.Pod, func() {
		streamErr := <-session.Done()
		_ = conn.send(ctx, api.ExecChannelError, endMessage(streamErr))
		cancel()
	})

	conn.pump(ctx, socket, session)
}

func endMessage(err error) []byte {
	if err == nil {
		return nil
	}
	return []byte(plainly(err))
}

func plainly(err error) string {
	text := err.Error()
	switch {
	case errors.Is(err, context.Canceled):
		return "the shell was closed"
	case brokenConnection(text):
		return "the connection to the cluster dropped, so the shell ended"
	case errors.Is(err, context.DeadlineExceeded):
		return "the cluster stopped answering, so the shell ended"
	}
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return "the cluster refused the shell: " + text
	}
	return text
}

func brokenConnection(text string) bool {
	for _, mark := range []string{
		"abnormal closure",
		"unexpected EOF",
		"use of closed network connection",
		"connection reset by peer",
		"broken pipe",
		"going away",
	} {
		if strings.Contains(text, mark) {
			return true
		}
	}
	return false
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
	safe.Go("writing to the terminal", func() {
		defer close(done)
		_, _ = session.Write(payload)
	})
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

type shellTally struct {
	mu      sync.Mutex
	open    int
	drained chan struct{}
}

func (t *shellTally) start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.open++
	if t.drained == nil {
		t.drained = make(chan struct{})
	}
}

func (t *shellTally) done() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.open--
	if t.open > 0 {
		return
	}
	if t.drained == nil {
		return
	}
	close(t.drained)
	t.drained = nil
}

func (t *shellTally) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.open
}

func (t *shellTally) waiter() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.drained != nil {
		return t.drained
	}
	gone := make(chan struct{})
	close(gone)
	return gone
}

func awaitNodeShells() {
	timer := time.NewTimer(nodeShellDrain)
	defer timer.Stop()
	select {
	case <-nodeShells.waiter():
		return
	case <-timer.C:
		slog.Warn("a node shell pod may still be running on the cluster", "waited", nodeShellDrain)
	}
}
