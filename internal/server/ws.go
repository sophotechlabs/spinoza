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
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	maxLogBatch                       = 200
	maxSubscriptions                  = 64
	defaultLiveConnectionLimit        = 128
	defaultIdentityConnectionLimit    = 8
	defaultFeedPingInterval           = 30 * time.Second
	defaultFeedPingTimeout            = 10 * time.Second
	defaultAuthorizationCheckInterval = 5 * time.Second
)

const msgError = "error"

var podCountInterval = 2 * time.Second

type relayStep int

const (
	relayLine relayStep = iota
	relayIdle
	relayEnd
	relayStop
)

const minResyncInterval = 2 * time.Second

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
	cluster  string
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	release, ok := s.claimLiveConnection(r)
	if !ok {
		writeError(w, http.StatusTooManyRequests, "too many live connections are already open")
		return
	}
	defer release()
	conn, err := accept(w, r)
	if err != nil {
		slog.Warn("a websocket upgrade was refused", "path", r.URL.Path, "error", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	sess := &wsSession{
		conn:      conn,
		ctx:       ctx,
		lookup:    s.lookup,
		tables:    map[string]*entry{},
		logs:      map[string]*entry{},
		pingEvery: s.feedPingEvery,
		pingWait:  s.feedPingWait,
	}
	defer sess.closeAll()
	kind := viewOf(r)
	s.track(sess)
	s.views.opened(kind)
	defer s.views.closed(kind)
	defer s.forget(sess)
	sess.write(ctx, s.contextFrame())
	for _, health := range s.healthOfEveryCluster() {
		sess.write(ctx, health)
	}
	s.watchCluster(ctx)
	safe.Go("watching a feed websocket", func() { sess.keepAlive(ctx, cancel) })
	s.revalidateLive(ctx, cancel, r, "revalidating a feed websocket")

	for {
		var msg api.ClientMsg
		if readErr := wsjson.Read(ctx, conn, &msg); readErr != nil {
			slog.Debug("a feed stopped reading", "error", readErr)
			return
		}
		sess.handle(msg)
	}
}

func liveIdentity(r *http.Request) string {
	who, ok := auth.IdentityFrom(r.Context())
	return identityName(who, ok)
}

func identityName(who auth.Identity, ok bool) string {
	if !ok {
		return "local"
	}
	if who.User != "" {
		return who.User
	}
	if who.Session != "" {
		return who.Session
	}
	return "anonymous"
}

func (s *Server) claimLiveConnection(r *http.Request) (func(), bool) {
	identity := liveIdentity(r)
	s.mu.Lock()
	liveLimit := s.liveLimit
	if liveLimit <= 0 {
		liveLimit = defaultLiveConnectionLimit
	}
	identityLimit := s.identityLimit
	if identityLimit <= 0 {
		identityLimit = defaultIdentityConnectionLimit
	}
	globalFull := s.live >= liveLimit
	identityFull := s.liveByUser[identity] >= identityLimit
	if globalFull || identityFull {
		globalOpen := s.live
		identityOpen := s.liveByUser[identity]
		s.mu.Unlock()
		slog.Warn("a live connection was refused because its budget is full",
			"identity", identity, "global_open", globalOpen, "identity_open", identityOpen)
		return nil, false
	}
	s.live++
	s.liveByUser[identity]++
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.live--
			s.liveByUser[identity]--
			if s.liveByUser[identity] == 0 {
				delete(s.liveByUser, identity)
			}
			s.mu.Unlock()
		})
	}, true
}

func (s *Server) revalidateLive(ctx context.Context, cancel context.CancelFunc, r *http.Request, task string) {
	who, known := auth.IdentityFrom(ctx)
	identity := identityName(who, known)
	safe.Go(task, func() { s.watchAuthorization(ctx, cancel, r, who, known, identity) })
}

func (s *Server) watchAuthorization(
	ctx context.Context,
	cancel context.CancelFunc,
	r *http.Request,
	who auth.Identity,
	known bool,
	identity string,
) {
	interval := s.authEvery
	if interval <= 0 {
		interval = defaultAuthorizationCheckInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			authn := s.authenticator()
			if authn == nil {
				continue
			}
			if known && authn.StillValid(r, who) {
				continue
			}
			slog.Info("a live connection ended because its authentication is no longer valid", "identity", identity)
			cancel()
			return
		}
	}
}

type wsSession struct {
	conn      *websocket.Conn
	ctx       context.Context
	lookup    clusterLookup
	mu        sync.Mutex
	tables    map[string]*entry
	logs      map[string]*entry
	nextGen   uint64
	writeMu   sync.Mutex
	pingEvery time.Duration
	pingWait  time.Duration
}

func (sess *wsSession) keepAlive(ctx context.Context, cancel context.CancelFunc) {
	ticker := time.NewTicker(sess.pingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, stop := context.WithTimeout(ctx, sess.pingWait)
			err := sess.conn.Ping(pingCtx)
			stop()
			if err == nil {
				continue
			}
			slog.Debug("a feed stopped answering", "error", err)
			cancel()
			return
		}
	}
}

func (sess *wsSession) entriesOf(which feed) map[string]*entry {
	if which == streams {
		return sess.logs
	}
	return sess.tables
}

func (sess *wsSession) tryClaim(which feed, subID, cluster string) (uint64, bool) {
	sess.mu.Lock()
	previous, existed := sess.entriesOf(which)[subID]
	if !existed && len(sess.tables)+len(sess.logs) >= maxSubscriptions {
		sess.mu.Unlock()
		return 0, false
	}
	sess.nextGen++
	gen := sess.nextGen
	sess.entriesOf(which)[subID] = &entry{gen: gen, cluster: cluster}
	sess.mu.Unlock()
	if existed {
		stop(previous)
	}
	return gen, true
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

func (sess *wsSession) failAndForget(which feed, subID string, gen uint64, err error) {
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	held, current := sess.forgetCurrent(which, subID, gen)
	if !current {
		return
	}
	stop(held)
	slog.Warn("a feed could not be opened", "subId", subID, "error", err)
	sess.writeLocked(sess.ctx, api.FeedError{Type: msgError, SubID: subID, Message: err.Error()})
}

func (sess *wsSession) forgetCurrent(which feed, subID string, gen uint64) (*entry, bool) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	held, ok := sess.entriesOf(which)[subID]
	if !ok || held.gen != gen {
		return nil, false
	}
	delete(sess.entriesOf(which), subID)
	return held, true
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
	case "more":
		sess.more(msg)
	case "logs-subscribe":
		sess.subscribeLogs(msg)
	case "logs-unsubscribe":
		sess.drop(streams, msg.SubID)
	default:
	}
}

func (sess *wsSession) subscribe(msg api.ClientMsg) {
	backend, on := sess.lookup(msg.Cluster)
	gen, claimed := sess.tryClaim(tables, msg.SubID, on)
	if !claimed {
		sess.refuse(msg.SubID)
		return
	}
	if backend == nil {
		sess.failAndForget(tables, msg.SubID, gen, notOpen(msg.Cluster))
		return
	}
	safe.Go("building the subscription "+msg.SubID, func() { sess.buildSub(backend, msg, gen) })
}

func (sess *wsSession) refuse(subID string) {
	sess.write(sess.ctx, api.FeedError{
		Type:    msgError,
		SubID:   subID,
		Message: "this connection already holds the maximum number of subscriptions",
	})
}

type resizable interface {
	SetLimit(limit int)
}

func (sess *wsSession) more(msg api.ClientMsg) {
	resource := sess.resourceOf(tables, msg.SubID)
	if resource == nil {
		return
	}
	sub, ok := resource.(resizable)
	if !ok {
		return
	}
	sub.SetLimit(msg.Limit)
}

func (sess *wsSession) resourceOf(which feed, subID string) stoppable {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	held, ok := sess.entriesOf(which)[subID]
	if !ok {
		return nil
	}
	return held.resource
}

func (sess *wsSession) buildSub(backend Reader, msg api.ClientMsg, gen uint64) {
	sub, err := backend.Subscribe(
		sess.ctx, msg.Group, msg.Version, msg.Resource, msg.Namespace, msg.Limit, msg.Filters,
	)
	if err != nil {
		sess.failAndForget(tables, msg.SubID, gen, err)
		return
	}
	if !sess.adopt(tables, msg.SubID, gen, sub) {
		sub.Close()
		return
	}
	sess.writeCurrent(tables, msg.SubID, gen, snapshotOf(msg.SubID, sub, sub.Rows, sub.Total))
	sess.relay(msg.SubID, gen, sub)
}

func snapshotOf(subID string, sub *resources.Subscription, rows []api.Row, total int) api.Snapshot {
	return api.Snapshot{
		Type:       "snapshot",
		SubID:      subID,
		Columns:    columnsOrEmpty(sub.Columns()),
		Namespaced: sub.Namespaced,
		Rows:       rowsOrEmpty(rows),
		Total:      total,
		Limit:      sub.Limit(),
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
	sub.Refresh(sess.ctx)
	rows, total, err := sub.Snapshot()
	if err != nil {
		slog.Warn("a resync could not read the cache", "subId", subID, "error", err)
		return sess.writeCurrent(tables, subID, gen, api.FeedError{Type: msgError, SubID: subID, Message: err.Error()})
	}
	return sess.writeCurrent(tables, subID, gen, snapshotOf(subID, sub, rows, total))
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
	backend, on := sess.lookup(msg.Cluster)
	gen, claimed := sess.tryClaim(streams, msg.SubID, on)
	if !claimed {
		sess.refuse(msg.SubID)
		return
	}
	if backend == nil {
		sess.failAndForget(streams, msg.SubID, gen, notOpen(msg.Cluster))
		return
	}
	safe.Go("opening the log stream "+msg.SubID, func() { sess.buildLogs(backend, msg, gen) })
}

func (sess *wsSession) buildLogs(backend Reader, msg api.ClientMsg, gen uint64) {
	selector, selErr := sess.selectorFor(backend, msg)
	if selErr != nil {
		sess.failAndForget(streams, msg.SubID, gen, selErr)
		return
	}
	stream, err := backend.Logs(sess.ctx, logs.Request{
		Namespace: msg.Namespace,
		Name:      msg.Name,
		Container: msg.Container,
		TailLines: msg.TailLines,
		Follow:    msg.Follow,
		Selector:  selector,
	})
	if err != nil {
		sess.failAndForget(streams, msg.SubID, gen, err)
		return
	}
	if !sess.adopt(streams, msg.SubID, gen, stream) {
		stream.Close()
		return
	}
	sess.writeCurrent(streams, msg.SubID, gen, openedBy(msg.SubID, stream))
	sess.relayLogs(msg.SubID, gen, stream)
}

func (sess *wsSession) selectorFor(backend Reader, msg api.ClientMsg) (string, error) {
	if msg.Resource == "" || msg.Resource == "pods" {
		return "", nil
	}
	return backend.PodSelector(sess.ctx, api.ObjectRef{
		Group:     msg.Group,
		Version:   msg.Version,
		Resource:  msg.Resource,
		Namespace: msg.Namespace,
		Name:      msg.Name,
	})
}

func (sess *wsSession) relayLogs(subID string, gen uint64, stream *logs.Stream) {
	ticker := time.NewTicker(podCountInterval)
	defer ticker.Stop()
	var held *logs.Line
	reported := openedBy(subID, stream)
	for {
		line, step := sess.nextLine(stream, held, ticker.C)
		if step == relayStop {
			return
		}
		if step == relayEnd {
			sess.writeCurrent(streams, subID, gen, endOfLogs(subID, stream))
			return
		}
		if step == relayLine {
			texts, leftover := batchLines(stream.Lines, *line)
			held = leftover
			batch := api.LogLines{Type: "log", SubID: subID, Lines: texts, Source: line.Pod}
			if !sess.writeCurrent(streams, subID, gen, batch) {
				return
			}
		}
		if !sess.reportPods(subID, gen, stream, &reported) {
			return
		}
	}
}

func openedBy(subID string, stream *logs.Stream) api.LogOpened {
	return api.LogOpened{
		Type:     "log-open",
		SubID:    subID,
		Attached: stream.Attached(),
		Matched:  stream.Matched(),
	}
}

func (sess *wsSession) reportPods(
	subID string,
	gen uint64,
	stream *logs.Stream,
	reported *api.LogOpened,
) bool {
	now := openedBy(subID, stream)
	if now == *reported {
		return true
	}
	*reported = now
	return sess.writeCurrent(streams, subID, gen, now)
}

func (sess *wsSession) nextLine(
	stream *logs.Stream,
	held *logs.Line,
	tick <-chan time.Time,
) (*logs.Line, relayStep) {
	if held != nil {
		return held, relayLine
	}
	select {
	case <-sess.ctx.Done():
		return nil, relayStop
	case <-tick:
		return nil, relayIdle
	case line, ok := <-stream.Lines:
		if !ok {
			return nil, relayEnd
		}
		return &line, relayLine
	}
}

func endOfLogs(subID string, stream *logs.Stream) any {
	err := stream.Err()
	if err == nil {
		return api.LogEnd{Type: "log-end", SubID: subID}
	}
	return api.FeedError{Type: msgError, SubID: subID, Message: err.Error()}
}

func batchLines(lines <-chan logs.Line, first logs.Line) ([]string, *logs.Line) {
	batch := []string{first.Text}
	for len(batch) < maxLogBatch {
		select {
		case line, ok := <-lines:
			if !ok {
				return batch, nil
			}
			if line.Pod != first.Pod {
				return batch, &line
			}
			batch = append(batch, line.Text)
		default:
			return batch, nil
		}
	}
	return batch, nil
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

func (s *Server) trackExec(conn *websocket.Conn, cluster string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminals[conn] = cluster
}

func (s *Server) terminalsOn(cluster string) []*websocket.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	open := make([]*websocket.Conn, 0, len(s.terminals))
	for conn, on := range s.terminals {
		if on == cluster {
			open = append(open, conn)
		}
	}
	return open
}

func (s *Server) forgetExec(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.terminals, conn)
}

func (s *Server) contextFrame() api.ContextChanged {
	return api.ContextChanged{
		Type:    "context",
		Cluster: s.cluster.ID(),
		Context: s.cluster.Contexts().Current.Name,
	}
}

func (s *Server) announceContext() {
	msg := s.contextFrame()
	for _, sess := range s.openSessions() {
		sess.write(sess.ctx, msg)
	}
}

func (s *Server) dropSubscriptionsOn(cluster string) {
	for _, sess := range s.openSessions() {
		sess.dropOn(cluster)
	}
}

func (sess *wsSession) dropOn(cluster string) {
	sess.mu.Lock()
	gone := make([]*entry, 0, len(sess.tables)+len(sess.logs))
	for _, which := range []feed{tables, streams} {
		for subID, held := range sess.entriesOf(which) {
			if held.cluster != cluster {
				continue
			}
			gone = append(gone, held)
			delete(sess.entriesOf(which), subID)
		}
	}
	sess.mu.Unlock()
	for _, held := range gone {
		stop(held)
	}
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
	s.terminals = map[*websocket.Conn]string{}
	s.mu.Unlock()
	for _, sess := range open {
		_ = sess.conn.CloseNow()
	}
	for _, conn := range shells {
		_ = conn.CloseNow()
	}
}
