package server

import (
	"context"
	"sync"
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

func (s *Server) watchCluster(ctx context.Context) {
	s.mu.Lock()
	if s.watching {
		s.mu.Unlock()
		return
	}
	s.watching = true
	s.mu.Unlock()
	outlives := context.WithoutCancel(ctx)
	safe.Go("watching whether the clusters answer", func() {
		s.pingUntilNobodyIsWatching(outlives)
	})
}

func (s *Server) pingUntilNobodyIsWatching(ctx context.Context) {
	ticker := time.NewTicker(s.pingInterval())
	defer ticker.Stop()
	watched := newSinkWatchers(s)
	defer watched.stopAll()
	s.pingEveryCluster(ctx)
	watched.follow(ctx, s.openClusterIDs())
	for {
		select {
		case <-ctx.Done():
			s.stopWatching()
			return
		case <-ticker.C:
			if s.sessionsOpen() == 0 {
				s.stopWatching()
				return
			}
			s.pingEveryCluster(ctx)
			watched.follow(ctx, s.openClusterIDs())
		}
	}
}

func (s *Server) stopWatching() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watching = false
}

func (s *Server) openClusterIDs() []string {
	opened := s.cluster.Opened()
	ids := make([]string, 0, len(opened))
	for _, one := range opened {
		ids = append(ids, one.ID)
	}
	return ids
}

func (s *Server) pingEveryCluster(ctx context.Context) {
	var asking sync.WaitGroup
	for _, id := range s.openClusterIDs() {
		asking.Add(1)
		safe.Go("asking whether "+id+" answers", func() {
			defer asking.Done()
			s.pingOne(ctx, id)
		})
	}
	asking.Wait()
}

func (s *Server) pingOne(ctx context.Context, id string) {
	backend := s.managerOf(id)
	if backend == nil {
		return
	}
	bounded, cancel := context.WithTimeout(ctx, clusterPingTimeout)
	defer cancel()
	s.recordPingOf(id, healthOf(backend.Ping(bounded)))
}

func (s *Server) sinkOf(id string) *reach.Sink {
	backend := s.managerOf(id)
	if backend == nil {
		return nil
	}
	return backend.Reach()
}

func (s *Server) sessionsOpen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
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

func assumedHealthOf(id string) api.ClusterHealth {
	health := answering()
	health.Cluster = id
	return health
}

const missesBeforeUnreachable = 3

func (s *Server) recordHealthOf(id string, now api.ClusterHealth) {
	now.Cluster = id
	s.publishHealthOf(id, s.settled(id, now))
}

func (s *Server) recordPingOf(id string, now api.ClusterHealth) {
	s.recordHealthOf(id, now)
}

func (s *Server) publishHealthOf(id string, now api.ClusterHealth) {
	was := assumedHealthOf(id)
	s.mu.Lock()
	held, known := s.health[id]
	if known {
		was = held
	}
	s.health[id] = now
	s.mu.Unlock()
	if was == now {
		return
	}
	s.announceHealthOf(id, now)
}

func (s *Server) settled(id string, now api.ClusterHealth) api.ClusterHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Reachable {
		delete(s.misses, id)
		return now
	}
	s.misses[id]++
	if s.misses[id] >= missesBeforeUnreachable {
		return now
	}
	return api.ClusterHealth{
		Type:      now.Type,
		Cluster:   now.Cluster,
		Reachable: true,
		Wobbling:  true,
		Reason:    now.Reason,
	}
}

func (s *Server) forgetHealthOf(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.health, id)
	delete(s.misses, id)
}

func (s *Server) healthOfCluster(id string) api.ClusterHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	held, known := s.health[id]
	if !known {
		return assumedHealthOf(id)
	}
	return held
}

func (s *Server) clusterHealth() api.ClusterHealth {
	return s.healthOfCluster(s.cluster.ID())
}

func (s *Server) announceHealthOf(_ string, health api.ClusterHealth) {
	for _, sess := range s.openSessions() {
		sess.write(sess.ctx, health)
	}
}

func (s *Server) healthOfEveryCluster() []api.ClusterHealth {
	open := s.openClusterIDs()
	if len(open) == 0 {
		return []api.ClusterHealth{s.clusterHealth()}
	}
	out := make([]api.ClusterHealth, 0, len(open))
	for _, id := range open {
		out = append(out, s.healthOfCluster(id))
	}
	return out
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
