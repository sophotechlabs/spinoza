package server

import (
	"context"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/reach"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const defaultPingInterval = 10 * time.Second

const clusterPingTimeout = 5 * time.Second

func (s *Server) pingInterval() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingEvery
}

// Inherits the first feed's context without dying with it.
func (s *Server) watchCluster(ctx context.Context) {
	s.mu.Lock()
	if s.watching {
		s.mu.Unlock()
		return
	}
	s.watching = true
	s.mu.Unlock()
	outlives := context.WithoutCancel(ctx)
	safe.Go("watching whether the cluster answers", func() {
		s.pingUntilNobodyIsWatching(outlives)
	})
}

// The timer covers a cluster nobody is asking anything of.
func (s *Server) pingUntilNobodyIsWatching(ctx context.Context) {
	ticker := time.NewTicker(s.pingInterval())
	defer ticker.Stop()
	s.pingCluster(ctx)
	for {
		select {
		case <-ticker.C:
			if s.sessionsOpen() == 0 {
				s.stopWatching()
				return
			}
			s.pingCluster(ctx)
		case <-s.reach().Changed():
			s.recordHealth(s.reachHealth())
		}
	}
}

func (s *Server) stopWatching() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watching = false
}

func (s *Server) reach() *reach.Sink {
	backend := s.manager()
	if backend == nil {
		return nil
	}
	return backend.Reach()
}

func (s *Server) reachHealth() api.ClusterHealth {
	alive, reason := s.reach().State()
	if alive {
		return answering()
	}
	return notAnswering(reason)
}

func (s *Server) sessionsOpen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *Server) pingCluster(ctx context.Context) {
	bounded, cancel := context.WithTimeout(ctx, clusterPingTimeout)
	defer cancel()
	s.recordHealth(healthOf(s.manager().Ping(bounded)))
}

func healthOf(err error) api.ClusterHealth {
	if err == nil {
		return answering()
	}
	return notAnswering(err.Error())
}

func answering() api.ClusterHealth {
	return api.ClusterHealth{Type: "cluster", Reachable: true}
}

func notAnswering(reason string) api.ClusterHealth {
	return api.ClusterHealth{Type: "cluster", Reachable: false, Reason: reason}
}

func (s *Server) recordHealth(now api.ClusterHealth) {
	s.mu.Lock()
	same := s.health == now
	s.health = now
	s.mu.Unlock()
	if same {
		return
	}
	s.announceHealth()
}

func assumedHealth() api.ClusterHealth {
	return answering()
}

func (s *Server) forgetHealth() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health = assumedHealth()
}

func (s *Server) clusterHealth() api.ClusterHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health
}

func (s *Server) announceHealth() {
	health := s.clusterHealth()
	for _, sess := range s.openSessions() {
		sess.write(sess.ctx, health)
	}
}

func (s *Server) openSessions() []*wsSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	open := make([]*wsSession, 0, len(s.sessions))
	for sess := range s.sessions {
		open = append(open, sess)
	}
	return open
}
