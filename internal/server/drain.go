package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/safe"
)

const drainReason = "spinoza is shutting down"

func (s *Server) Drain(ctx context.Context) {
	s.drainBegin()
	s.drainFeeds()
	if s.drainQuiet(ctx) {
		return
	}
	s.dropSessions()
}

func (s *Server) drainBegin() {
	s.mu.Lock()
	s.shuttingDown = true
	s.mu.Unlock()
}

func (s *Server) draining() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shuttingDown
}

const tooManyLive = "too many live connections are already open"

func (s *Server) admitLive(w http.ResponseWriter, r *http.Request) (func(), bool) {
	if s.draining() {
		writeError(w, http.StatusServiceUnavailable, drainReason)
		return nil, false
	}
	release, allowed := s.claimLiveConnection(r)
	if !allowed {
		writeError(w, http.StatusTooManyRequests, tooManyLive)
		return nil, false
	}
	return release, true
}

func (s *Server) drainFeeds() {
	var closing sync.WaitGroup
	for _, sess := range s.openSessions() {
		closing.Add(1)
		safe.Go("closing a feed while shutting down", func() {
			defer closing.Done()
			_ = sess.conn.Close(websocket.StatusServiceRestart, drainReason)
		})
	}
	closing.Wait()
}

func (s *Server) drainQuiet(ctx context.Context) bool {
	for {
		if s.drainedAlready() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(drainStep):
		}
	}
}

func (s *Server) drainedAlready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions) == 0 && len(s.terminals) == 0
}
