package server

import (
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/broker"
)

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()

	ctx := r.Context()
	events, cancel := s.broker.Subscribe()
	defer cancel()

	rows, rv := s.broker.Snapshot()
	snap := api.ServerMsg{Type: "snapshot", Resource: "pods", Items: rows, RV: rv}
	if err := wsjson.Write(ctx, c, snap); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if err := wsjson.Write(ctx, c, eventToMsg(ev)); err != nil {
				return
			}
		}
	}
}

func eventToMsg(ev broker.Event) api.ServerMsg {
	if ev.Kind == "deleted" {
		return api.ServerMsg{Type: "deleted", Resource: "pods", UID: ev.UID}
	}
	row := ev.Row
	return api.ServerMsg{Type: ev.Kind, Resource: "pods", Item: &row}
}
