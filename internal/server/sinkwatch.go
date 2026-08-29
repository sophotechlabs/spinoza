package server

import (
	"context"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/reach"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

type sinkWatchers struct {
	server *Server

	mu   sync.Mutex
	held map[string]chan struct{}
}

func newSinkWatchers(server *Server) *sinkWatchers {
	return &sinkWatchers{server: server, held: map[string]chan struct{}{}}
}

func (w *sinkWatchers) follow(ctx context.Context, ids []string) {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, stop := range w.held {
		if wanted[id] {
			continue
		}
		close(stop)
		delete(w.held, id)
	}
	for _, id := range ids {
		if w.held[id] != nil {
			continue
		}
		sink := w.server.sinkOf(id)
		if sink == nil {
			continue
		}
		stop := make(chan struct{})
		w.held[id] = stop
		safe.Go("watching whether "+id+" answers", func() {
			w.server.followSink(ctx, stop, id, sink)
		})
	}
}

func (w *sinkWatchers) watching() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.held)
}

func (w *sinkWatchers) stopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, stop := range w.held {
		close(stop)
		delete(w.held, id)
	}
}

func (s *Server) followSink(ctx context.Context, stop <-chan struct{}, id string, sink *reach.Sink) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-sink.Changed():
			alive, reason := sink.State()
			if alive {
				s.recordHealthOf(id, answering())
				continue
			}
			s.recordHealthOf(id, notAnswering(reason))
		}
	}
}
