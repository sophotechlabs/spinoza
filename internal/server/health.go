package server

import (
	"context"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

// defaultPingInterval is how often spinoza checks that the cluster still
// answers. The prober only runs while a window is watching.
const defaultPingInterval = 10 * time.Second

const clusterPingTimeout = 5 * time.Second

func (s *Server) pingInterval() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingEvery
}

// watchCluster keeps one prober running for as long as there is a window to
// tell. It starts on the first feed and stops when the last one goes, so it
// inherits that feed's context without dying with it.
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

func (s *Server) pingUntilNobodyIsWatching(ctx context.Context) {
	ticker := time.NewTicker(s.pingInterval())
	defer ticker.Stop()
	s.pingCluster(ctx)
	for range ticker.C {
		if s.sessionsOpen() == 0 {
			s.mu.Lock()
			s.watching = false
			s.mu.Unlock()
			return
		}
		s.pingCluster(ctx)
	}
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
		return api.ClusterHealth{Type: "cluster", Reachable: true}
	}
	return api.ClusterHealth{Type: "cluster", Reachable: false, Reason: err.Error()}
}

// recordHealth keeps the answer and tells every window when it changed. An
// unchanged answer is not news.
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

// assumedHealth is what spinoza says before it has asked anything. A window
// that has just opened should not be told the cluster is down on no evidence.
func assumedHealth() api.ClusterHealth {
	return api.ClusterHealth{Type: "cluster", Reachable: true}
}

// forgetHealth drops what was known about the cluster that was current, so the
// next window is not told about a different one.
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
