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
	c, err := accept(w, r)
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	sess := &wsSession{
		conn: c,
		ctx:  ctx,
		mgr:  s.mgr,
		subs: map[string]*resources.Subscription{},
		logs: map[string]*logs.Stream{},
	}
	defer sess.closeAll()

	for {
		var msg api.ClientMsg
		if readErr := wsjson.Read(ctx, c, &msg); readErr != nil {
			return
		}
		sess.handle(msg)
	}
}

type wsSession struct {
	conn *websocket.Conn
	ctx  context.Context
	mgr  *resources.Manager
	mu   sync.Mutex
	subs map[string]*resources.Subscription
	logs map[string]*logs.Stream
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
	sess.subs[msg.SubID] = sub
	sess.mu.Unlock()

	sess.write(sess.ctx, api.ServerMsg{
		Type:       "snapshot",
		SubID:      msg.SubID,
		Columns:    sub.Columns,
		Namespaced: sub.Namespaced,
		Rows:       sub.Rows,
	})

	go sess.relay(msg.SubID, sub)
}

func (sess *wsSession) relay(subID string, sub *resources.Subscription) {
	for {
		select {
		case <-sess.ctx.Done():
			return
		case ev, ok := <-sub.Events:
			if !ok {
				return
			}
			sess.write(sess.ctx, eventToMsg(subID, ev))
		}
	}
}

func (sess *wsSession) unsubscribe(subID string) {
	sess.mu.Lock()
	sub, ok := sess.subs[subID]
	if ok {
		delete(sess.subs, subID)
	}
	sess.mu.Unlock()
	if ok {
		sub.Close()
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
	for _, sub := range sess.subs {
		subs = append(subs, sub)
	}
	streams := make([]*logs.Stream, 0, len(sess.logs))
	for _, stream := range sess.logs {
		streams = append(streams, stream)
	}
	sess.subs = map[string]*resources.Subscription{}
	sess.logs = map[string]*logs.Stream{}
	sess.mu.Unlock()
	for _, sub := range subs {
		sub.Close()
	}
	for _, stream := range streams {
		stream.Close()
	}
}

func (sess *wsSession) write(ctx context.Context, msg api.ServerMsg) {
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = wsjson.Write(writeCtx, sess.conn, msg)
}

func eventToMsg(subID string, ev resources.Event) api.ServerMsg {
	if ev.Kind == "deleted" {
		return api.ServerMsg{Type: "deleted", SubID: subID, UID: ev.UID}
	}
	row := ev.Row
	return api.ServerMsg{Type: ev.Kind, SubID: subID, Row: &row}
}
