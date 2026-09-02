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
	held map[string]sinkWatch
}

type sinkWatch struct {
	sink *reach.Sink
	stop chan struct{}
}

func newSinkWatchers(server *Server) *sinkWatchers {
	return &sinkWatchers{server: server, held: map[string]sinkWatch{}}
}

func (w *sinkWatchers) follow(ctx context.Context, ids []string) {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, watch := range w.held {
		if wanted[id] {
			continue
		}
		close(watch.stop)
		delete(w.held, id)
	}
	for _, id := range ids {
		sink := w.server.sinkOf(id)
		current, watched := w.held[id]
		if watched && current.sink == sink {
			continue
		}
		if watched {
			close(current.stop)
			delete(w.held, id)
		}
		if sink == nil {
			continue
		}
		stop := make(chan struct{})
		generation := w.server.healthGeneration(id)
		w.held[id] = sinkWatch{sink: sink, stop: stop}
		safe.Go("watching whether "+id+" answers", func() {
			w.server.followSink(ctx, stop, id, generation, sink)
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
	for id, watch := range w.held {
		close(watch.stop)
		delete(w.held, id)
	}
}

func (s *Server) followSink(
	ctx context.Context,
	stop <-chan struct{},
	id string,
	generation uint64,
	sink *reach.Sink,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-sink.Changed():
			alive, reason := sink.State()
			if alive {
				s.recordHealthAt(id, generation, answering())
				continue
			}
			s.recordHealthAt(id, generation, notAnswering(reason))
		}
	}
}
