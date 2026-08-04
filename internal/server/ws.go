package server

import (
	"context"
	"log/slog"
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

const msgError = "error"

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := accept(w, r)
	if err != nil {
		slog.Warn("a websocket upgrade was refused", "path", r.URL.Path, "error", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	sess := &wsSession{
		conn: conn,
		ctx:  ctx,
		subs: map[string]*subEntry{},
		logs: map[string]*logEntry{},
	}
	defer sess.closeAll()
	s.track(sess)
	defer s.forget(sess)
	sess.mgr = s.manager()

	for {
		var msg api.ClientMsg
		if readErr := wsjson.Read(ctx, conn, &msg); readErr != nil {
			slog.Debug("a feed stopped reading", "error", readErr)
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
	logs    map[string]*logEntry
	nextGen uint64
	writeMu sync.Mutex
}

type subEntry struct {
	sub *resources.Subscription
	gen uint64
}

type logEntry struct {
	stream *logs.Stream
	gen    uint64
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
	gen := sess.claim(msg.SubID)
	go sess.buildSub(msg, gen)
}

func (sess *wsSession) claim(subID string) uint64 {
	sess.mu.Lock()
	previous, existed := sess.subs[subID]
	sess.nextGen++
	gen := sess.nextGen
	sess.subs[subID] = &subEntry{gen: gen}
	sess.mu.Unlock()
	if existed {
		closeSub(previous)
	}
	return gen
}

func closeSub(entry *subEntry) {
	if entry.sub == nil {
		return
	}
	entry.sub.Close()
}

func (sess *wsSession) buildSub(msg api.ClientMsg, gen uint64) {
	sub, err := sess.mgr.Subscribe(msg.Group, msg.Version, msg.Resource, msg.Namespace)
	if err != nil {
		sess.failCurrent(msg.SubID, gen, err)
		return
	}
	if !sess.adopt(msg.SubID, gen, sub) {
		sub.Close()
		return
	}
	sess.writeCurrent(msg.SubID, gen, snapshotOf(msg.SubID, sub, sub.Rows))
	sess.relay(msg.SubID, gen, sub)
}

func (sess *wsSession) failCurrent(subID string, gen uint64, err error) {
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	if !sess.isCurrent(subID, gen) {
		return
	}
	slog.Warn("a subscription could not be built", "subId", subID, "error", err)
	sess.writeLocked(sess.ctx, api.ServerMsg{Type: msgError, SubID: subID, Message: err.Error()})
}

func (sess *wsSession) adopt(subID string, gen uint64, sub *resources.Subscription) bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	entry, ok := sess.subs[subID]
	if !ok {
		return false
	}
	if entry.gen != gen {
		return false
	}
	entry.sub = sub
	return true
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
	rows, err := sub.Snapshot()
	if err != nil {
		slog.Warn("a resync could not read the cache", "subId", subID, "error", err)
		return sess.writeCurrent(subID, gen, api.ServerMsg{Type: msgError, SubID: subID, Message: err.Error()})
	}
	return sess.writeCurrent(subID, gen, snapshotOf(subID, sub, rows))
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
		closeSub(entry)
	}
}

func (sess *wsSession) subscribeLogs(msg api.ClientMsg) {
	gen := sess.claimLogs(msg.SubID)
	go sess.buildLogs(msg, gen)
}

func (sess *wsSession) claimLogs(subID string) uint64 {
	sess.mu.Lock()
	previous, existed := sess.logs[subID]
	sess.nextGen++
	gen := sess.nextGen
	sess.logs[subID] = &logEntry{gen: gen}
	sess.mu.Unlock()
	if existed {
		closeStream(previous)
	}
	return gen
}

func closeStream(entry *logEntry) {
	if entry.stream == nil {
		return
	}
	entry.stream.Close()
}

func (sess *wsSession) buildLogs(msg api.ClientMsg, gen uint64) {
	stream, err := sess.mgr.Logs(sess.ctx, logs.Request{
		Namespace: msg.Namespace,
		Name:      msg.Name,
		Container: msg.Container,
		TailLines: msg.TailLines,
		Follow:    msg.Follow,
	})
	if err != nil {
		sess.failCurrentLogs(msg.SubID, gen, err)
		return
	}
	if !sess.adoptLogs(msg.SubID, gen, stream) {
		stream.Close()
		return
	}
	sess.relayLogs(msg.SubID, gen, stream)
}

func (sess *wsSession) failCurrentLogs(subID string, gen uint64, err error) {
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	if !sess.isCurrentLogs(subID, gen) {
		return
	}
	slog.Warn("a log stream could not be opened", "subId", subID, "error", err)
	sess.writeLocked(sess.ctx, api.ServerMsg{Type: msgError, SubID: subID, Message: err.Error()})
}

func (sess *wsSession) isCurrentLogs(subID string, gen uint64) bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	entry, ok := sess.logs[subID]
	if !ok {
		return false
	}
	return entry.gen == gen
}

func (sess *wsSession) adoptLogs(subID string, gen uint64, stream *logs.Stream) bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	entry, ok := sess.logs[subID]
	if !ok {
		return false
	}
	if entry.gen != gen {
		return false
	}
	entry.stream = stream
	return true
}

func (sess *wsSession) relayLogs(subID string, gen uint64, stream *logs.Stream) {
	for {
		select {
		case <-sess.ctx.Done():
			return
		case line, ok := <-stream.Lines:
			if !ok {
				sess.writeCurrentLogs(subID, gen, endOfLogs(subID, stream))
				return
			}
			batch := api.ServerMsg{Type: "log", SubID: subID, Lines: batchLines(stream.Lines, line)}
			if !sess.writeCurrentLogs(subID, gen, batch) {
				return
			}
		}
	}
}

func endOfLogs(subID string, stream *logs.Stream) api.ServerMsg {
	err := stream.Err()
	if err == nil {
		return api.ServerMsg{Type: "log-end", SubID: subID}
	}
	return api.ServerMsg{Type: msgError, SubID: subID, Message: err.Error()}
}

func (sess *wsSession) writeCurrentLogs(subID string, gen uint64, msg any) bool {
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	if !sess.isCurrentLogs(subID, gen) {
		return false
	}
	sess.writeLocked(sess.ctx, msg)
	return true
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
	entry, ok := sess.logs[subID]
	if ok {
		delete(sess.logs, subID)
	}
	sess.mu.Unlock()
	if ok {
		closeStream(entry)
	}
}

func (sess *wsSession) closeAll() {
	sess.mu.Lock()
	subs := make([]*subEntry, 0, len(sess.subs))
	for _, entry := range sess.subs {
		subs = append(subs, entry)
	}
	streams := make([]*logEntry, 0, len(sess.logs))
	for _, entry := range sess.logs {
		streams = append(streams, entry)
	}
	sess.subs = map[string]*subEntry{}
	sess.logs = map[string]*logEntry{}
	sess.mu.Unlock()
	for _, entry := range subs {
		closeSub(entry)
	}
	for _, entry := range streams {
		closeStream(entry)
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
	err := wsjson.Write(writeCtx, sess.conn, msg)
	if err != nil {
		slog.Warn("a feed frame could not be delivered", "error", err)
	}
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
	if ev.Kind == msgError {
		return api.ServerMsg{Type: msgError, SubID: subID, Message: ev.Message}
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

func (s *Server) trackExec(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminals[conn] = struct{}{}
}

func (s *Server) forgetExec(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.terminals, conn)
}

func (s *Server) dropSessions() {
	s.mu.Lock()
	open := make([]*wsSession, 0, len(s.sessions))
	for sess := range s.sessions {
		open = append(open, sess)
	}
	shells := make([]*websocket.Conn, 0, len(s.terminals))
	for conn := range s.terminals {
		shells = append(shells, conn)
	}
	s.sessions = map[*wsSession]struct{}{}
	s.terminals = map[*websocket.Conn]struct{}{}
	s.mu.Unlock()
	for _, sess := range open {
		_ = sess.conn.Close(websocket.StatusGoingAway, "context changed")
	}
	for _, conn := range shells {
		_ = conn.Close(websocket.StatusGoingAway, "context changed")
	}
}
