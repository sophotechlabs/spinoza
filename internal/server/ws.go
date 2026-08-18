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
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const maxLogBatch = 200

const msgError = "error"

var minResyncInterval = 2 * time.Second

type throttle struct {
	interval time.Duration
	now      func() time.Time
	last     time.Time
}

func newThrottle(interval time.Duration) *throttle {
	return &throttle{interval: interval, now: time.Now}
}

func (t *throttle) wait(ctx context.Context) bool {
	remaining := t.interval - t.now().Sub(t.last)
	if remaining <= 0 {
		t.last = t.now()
		return true
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		t.last = t.now()
		return true
	}
}

type feed int

const (
	tables feed = iota
	streams
)

type stoppable interface {
	Close()
}

type entry struct {
	resource stoppable
	gen      uint64
}

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
		conn:   conn,
		ctx:    ctx,
		tables: map[string]*entry{},
		logs:   map[string]*entry{},
	}
	defer sess.closeAll()
	kind := viewOf(r)
	s.track(sess)
	s.views.opened(kind)
	defer s.views.closed(kind)
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
	mgr     Backend
	mu      sync.Mutex
	tables  map[string]*entry
	logs    map[string]*entry
	nextGen uint64
	writeMu sync.Mutex
}

func (sess *wsSession) entriesOf(which feed) map[string]*entry {
	if which == streams {
		return sess.logs
	}
	return sess.tables
}

func (sess *wsSession) claim(which feed, subID string) uint64 {
	sess.mu.Lock()
	previous, existed := sess.entriesOf(which)[subID]
	sess.nextGen++
	gen := sess.nextGen
	sess.entriesOf(which)[subID] = &entry{gen: gen}
	sess.mu.Unlock()
	if existed {
		stop(previous)
	}
	return gen
}

func (sess *wsSession) adopt(which feed, subID string, gen uint64, resource stoppable) bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	held, ok := sess.entriesOf(which)[subID]
	if !ok {
		return false
	}
	if held.gen != gen {
		return false
	}
	held.resource = resource
	return true
}

func (sess *wsSession) isCurrent(which feed, subID string, gen uint64) bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	held, ok := sess.entriesOf(which)[subID]
	if !ok {
		return false
	}
	return held.gen == gen
}

func (sess *wsSession) writeCurrent(which feed, subID string, gen uint64, msg any) bool {
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	if !sess.isCurrent(which, subID, gen) {
		return false
	}
	sess.writeLocked(sess.ctx, msg)
	return true
}

func (sess *wsSession) failCurrent(which feed, subID string, gen uint64, err error) {
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	if !sess.isCurrent(which, subID, gen) {
		return
	}
	slog.Warn("a feed could not be opened", "subId", subID, "error", err)
	sess.writeLocked(sess.ctx, api.FeedError{Type: msgError, SubID: subID, Message: err.Error()})
}

func (sess *wsSession) drop(which feed, subID string) {
	sess.mu.Lock()
	held, ok := sess.entriesOf(which)[subID]
	if ok {
		delete(sess.entriesOf(which), subID)
	}
	sess.mu.Unlock()
	if ok {
		stop(held)
	}
}

func stop(held *entry) {
	if held.resource == nil {
		return
	}
	held.resource.Close()
}

func (sess *wsSession) handle(msg api.ClientMsg) {
	switch msg.Type {
	case "subscribe":
		sess.subscribe(msg)
	case "unsubscribe":
		sess.drop(tables, msg.SubID)
	case "logs-subscribe":
		sess.subscribeLogs(msg)
	case "logs-unsubscribe":
		sess.drop(streams, msg.SubID)
	default:
	}
}

func (sess *wsSession) subscribe(msg api.ClientMsg) {
	gen := sess.claim(tables, msg.SubID)
	safe.Go("building the subscription "+msg.SubID, func() { sess.buildSub(msg, gen) })
}

func (sess *wsSession) buildSub(msg api.ClientMsg, gen uint64) {
	sub, err := sess.mgr.Subscribe(sess.ctx, msg.Group, msg.Version, msg.Resource, msg.Namespace)
	if err != nil {
		sess.failCurrent(tables, msg.SubID, gen, err)
		return
	}
	if !sess.adopt(tables, msg.SubID, gen, sub) {
		sub.Close()
		return
	}
	sess.writeCurrent(tables, msg.SubID, gen, snapshotOf(msg.SubID, sub, sub.Rows))
	sess.relay(msg.SubID, gen, sub)
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
	spacing := newThrottle(minResyncInterval)
	for {
		select {
		case <-sess.ctx.Done():
			return
		case _, ok := <-sub.Resync:
			if !ok {
				return
			}
			if !spacing.wait(sess.ctx) {
				return
			}
			if !sess.sendResync(subID, gen, sub) {
				return
			}
		case ev, ok := <-sub.Events:
			if !ok {
				return
			}
			if !sess.writeBatch(subID, gen, ev, sub.Events) {
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
		return sess.writeCurrent(tables, subID, gen, api.FeedError{Type: msgError, SubID: subID, Message: err.Error()})
	}
	return sess.writeCurrent(tables, subID, gen, snapshotOf(subID, sub, rows))
}

const maxBatch = 200

func (sess *wsSession) writeBatch(
	subID string,
	gen uint64,
	first resources.Event,
	more <-chan resources.Event,
) bool {
	if first.Kind == msgError {
		return sess.writeCurrent(tables, subID, gen, eventToMsg(subID, first))
	}
	changes := []api.RowChange{changeOf(first)}
	for len(changes) < maxBatch {
		select {
		case next, ok := <-more:
			if !ok {
				return sess.writeCurrent(tables, subID, gen, batchOf(subID, changes))
			}
			if next.Kind == msgError {
				if !sess.writeCurrent(tables, subID, gen, batchOf(subID, changes)) {
					return false
				}
				return sess.writeCurrent(tables, subID, gen, eventToMsg(subID, next))
			}
			changes = append(changes, changeOf(next))
		default:
			return sess.writeCurrent(tables, subID, gen, batchOf(subID, changes))
		}
	}
	return sess.writeCurrent(tables, subID, gen, batchOf(subID, changes))
}

func batchOf(subID string, changes []api.RowChange) api.RowBatch {
	return api.RowBatch{Type: "batch", SubID: subID, Changes: changes}
}

func changeOf(ev resources.Event) api.RowChange {
	if ev.Kind == "deleted" {
		return api.RowChange{Type: "deleted", UID: ev.UID}
	}
	return api.RowChange{Type: ev.Kind, Row: ev.Row}
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

func (sess *wsSession) subscribeLogs(msg api.ClientMsg) {
	gen := sess.claim(streams, msg.SubID)
	safe.Go("opening the log stream "+msg.SubID, func() { sess.buildLogs(msg, gen) })
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
		sess.failCurrent(streams, msg.SubID, gen, err)
		return
	}
	if !sess.adopt(streams, msg.SubID, gen, stream) {
		stream.Close()
		return
	}
	sess.relayLogs(msg.SubID, gen, stream)
}

func (sess *wsSession) relayLogs(subID string, gen uint64, stream *logs.Stream) {
	for {
		select {
		case <-sess.ctx.Done():
			return
		case line, ok := <-stream.Lines:
			if !ok {
				sess.writeCurrent(streams, subID, gen, endOfLogs(subID, stream))
				return
			}
			batch := api.LogLines{Type: "log", SubID: subID, Lines: batchLines(stream.Lines, line)}
			if !sess.writeCurrent(streams, subID, gen, batch) {
				return
			}
		}
	}
}

func endOfLogs(subID string, stream *logs.Stream) any {
	err := stream.Err()
	if err == nil {
		return api.LogEnd{Type: "log-end", SubID: subID}
	}
	return api.FeedError{Type: msgError, SubID: subID, Message: err.Error()}
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

func (sess *wsSession) closeAll() {
	sess.mu.Lock()
	held := make([]*entry, 0, len(sess.tables)+len(sess.logs))
	for _, open := range sess.tables {
		held = append(held, open)
	}
	for _, open := range sess.logs {
		held = append(held, open)
	}
	sess.tables = map[string]*entry{}
	sess.logs = map[string]*entry{}
	sess.mu.Unlock()
	for _, open := range held {
		stop(open)
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

func eventToMsg(subID string, ev resources.Event) any {
	if ev.Kind == "deleted" {
		return api.RowDeleted{Type: "deleted", SubID: subID, UID: ev.UID}
	}
	if ev.Kind == msgError {
		return api.FeedError{Type: msgError, SubID: subID, Message: ev.Message}
	}
	return api.RowChanged{Type: ev.Kind, SubID: subID, Row: ev.Row}
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
