package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const maxLogBatch = 200

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := accept(w, r)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	sess := &wsSession{
		conn: conn,
		ctx:  ctx,
		mgr:  s.manager(),
		subs: map[string]*subEntry{},
		logs: map[string]*logs.Stream{},
	}
	defer sess.closeAll()
	s.track(sess)
	defer s.forget(sess)

	for {
		var msg api.ClientMsg
		if readErr := wsjson.Read(ctx, conn, &msg); readErr != nil {
			return
		}
		sess.handle(msg)
	}
}

type wsSession struct {
	conn    *websocket.Conn
	ctx     context.Context
	mgr     *resources.Manager
	mu      sync.Mutex
	subs    map[string]*subEntry
	logs    map[string]*logs.Stream
	nextGen uint64
	writeMu sync.Mutex
}

type subEntry struct {
	sub *resources.Subscription
	gen uint64
}

func (sess *wsSession) handle(msg api.ClientMsg) {
	switch msg.Type {
	case "subscribe":
		sess.subscribe(msg)
	case "unsubscribe":
		sess.unsubscribe(msg.SubID)
	case "logs-subscribe":
		sess.subscribeLogs(msg)
	case "logs-unsubscribe":
		sess.unsubscribeLogs(msg.SubID)
	default:
	}
}

func (sess *wsSession) subscribe(msg api.ClientMsg) {
	sess.unsubscribe(msg.SubID)
	sub, err := sess.mgr.Subscribe(msg.Group, msg.Version, msg.Resource, msg.Namespace)
	if err != nil {
		sess.write(sess.ctx, api.ServerMsg{Type: "error", SubID: msg.SubID, Message: err.Error()})
		return
	}
	sess.mu.Lock()
	sess.nextGen++
	gen := sess.nextGen
	sess.subs[msg.SubID] = &subEntry{sub: sub, gen: gen}
	sess.mu.Unlock()

	sess.writeCurrent(msg.SubID, gen, snapshotOf(msg.SubID, sub, sub.Rows))

	go sess.relay(msg.SubID, gen, sub)
}

func snapshotOf(subID string, sub *resources.Subscription, rows []api.Row) api.Snapshot {
	return api.Snapshot{
		Type:       "snapshot",
		SubID:      subID,
		Columns:    columnsOrEmpty(sub.Columns),
		Namespaced: sub.Namespaced,
		Rows:       rowsOrEmpty(rows),
	}
}

func (sess *wsSession) relay(subID string, gen uint64, sub *resources.Subscription) {
	for {
		select {
		case <-sess.ctx.Done():
			return
		case _, ok := <-sub.Resync:
			if !ok {
				return
			}
			if !sess.sendResync(subID, gen, sub) {
				return
			}
		case ev, ok := <-sub.Events:
			if !ok {
				return
			}
			if !sess.writeCurrent(subID, gen, eventToMsg(subID, ev)) {
				return
			}
		}
	}
}

func (sess *wsSession) sendResync(subID string, gen uint64, sub *resources.Subscription) bool {
	drainEvents(sub.Events)
	return sess.writeCurrent(subID, gen, snapshotOf(subID, sub, sub.Snapshot()))
}

func drainEvents(events <-chan resources.Event) {
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func (sess *wsSession) writeCurrent(subID string, gen uint64, msg any) bool {
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	if !sess.isCurrent(subID, gen) {
		return false
	}
	sess.writeLocked(sess.ctx, msg)
	return true
}

func (sess *wsSession) isCurrent(subID string, gen uint64) bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	entry, ok := sess.subs[subID]
	if !ok {
		return false
	}
	return entry.gen == gen
}

func (sess *wsSession) unsubscribe(subID string) {
	sess.mu.Lock()
	entry, ok := sess.subs[subID]
	if ok {
		delete(sess.subs, subID)
	}
	sess.mu.Unlock()
	if ok {
		entry.sub.Close()
	}
}

func (sess *wsSession) subscribeLogs(msg api.ClientMsg) {
	sess.unsubscribeLogs(msg.SubID)
	stream, err := sess.mgr.Logs(sess.ctx, logs.Request{
		Namespace: msg.Namespace,
		Name:      msg.Name,
		Container: msg.Container,
		TailLines: msg.TailLines,
		Follow:    msg.Follow,
	})
	if err != nil {
		sess.write(sess.ctx, api.ServerMsg{Type: "error", SubID: msg.SubID, Message: err.Error()})
		return
	}
	sess.mu.Lock()
	sess.logs[msg.SubID] = stream
	sess.mu.Unlock()

	go sess.relayLogs(msg.SubID, stream)
}

func (sess *wsSession) relayLogs(subID string, stream *logs.Stream) {
	for {
		select {
		case <-sess.ctx.Done():
			return
		case line, ok := <-stream.Lines:
			if !ok {
				sess.write(sess.ctx, api.ServerMsg{Type: "log-end", SubID: subID})
				return
			}
			sess.write(sess.ctx, api.ServerMsg{Type: "log", SubID: subID, Lines: batchLines(stream.Lines, line)})
		}
	}
}

func batchLines(lines <-chan string, first string) []string {
	batch := []string{first}
	for len(batch) < maxLogBatch {
		select {
		case line, ok := <-lines:
			if !ok {
				return batch
			}
			batch = append(batch, line)
		default:
			return batch
		}
	}
	return batch
}

func (sess *wsSession) unsubscribeLogs(subID string) {
	sess.mu.Lock()
	stream, ok := sess.logs[subID]
	if ok {
		delete(sess.logs, subID)
	}
	sess.mu.Unlock()
	if ok {
		stream.Close()
	}
}

func (sess *wsSession) closeAll() {
	sess.mu.Lock()
	subs := make([]*resources.Subscription, 0, len(sess.subs))
	for _, entry := range sess.subs {
		subs = append(subs, entry.sub)
	}
	streams := make([]*logs.Stream, 0, len(sess.logs))
	for _, stream := range sess.logs {
		streams = append(streams, stream)
	}
	sess.subs = map[string]*subEntry{}
	sess.logs = map[string]*logs.Stream{}
	sess.mu.Unlock()
	for _, sub := range subs {
		sub.Close()
	}
	for _, stream := range streams {
		stream.Close()
	}
}

func (sess *wsSession) write(ctx context.Context, msg any) {
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	sess.writeLocked(ctx, msg)
}

func (sess *wsSession) writeLocked(ctx context.Context, msg any) {
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = wsjson.Write(writeCtx, sess.conn, msg)
}

func columnsOrEmpty(columns []api.Column) []api.Column {
	if columns == nil {
		return []api.Column{}
	}
	return columns
}

func rowsOrEmpty(rows []api.Row) []api.Row {
	if rows == nil {
		return []api.Row{}
	}
	return rows
}

func eventToMsg(subID string, ev resources.Event) api.ServerMsg {
	if ev.Kind == "deleted" {
		return api.ServerMsg{Type: "deleted", SubID: subID, UID: ev.UID}
	}
	row := ev.Row
	return api.ServerMsg{Type: ev.Kind, SubID: subID, Row: &row}
}

func (s *Server) track(sess *wsSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess] = struct{}{}
}

func (s *Server) forget(sess *wsSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sess)
}

func (s *Server) dropSessions() {
	s.mu.Lock()
	open := make([]*wsSession, 0, len(s.sessions))
	for sess := range s.sessions {
		open = append(open, sess)
	}
	s.sessions = map[*wsSession]struct{}{}
	s.mu.Unlock()
	for _, sess := range open {
		_ = sess.conn.Close(websocket.StatusGoingAway, "context changed")
	}
}
