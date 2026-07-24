package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type Event struct {
	Kind string
	Row  api.PodRow
	UID  string
}

type Broker interface {
	Snapshot() ([]api.PodRow, string)
	Subscribe() (<-chan Event, func())
}

type stub struct {
	mu      sync.Mutex
	rows    map[string]api.PodRow
	subs    map[chan Event]struct{}
	rv      int
	counter int
}

func NewStub(ctx context.Context) Broker {
	return newStub(ctx, 4*time.Second)
}

func newStub(ctx context.Context, interval time.Duration) *stub {
	s := &stub{
		rows: map[string]api.PodRow{},
		subs: map[chan Event]struct{}{},
	}
	seed := []string{"coredns-6799f", "traefik-5d78b", "metrics-server-7c9", "local-path-prov-2", "spinoza-demo"}
	now := time.Now().UTC()
	for i, name := range seed {
		uid := fmt.Sprintf("stub-%d", i)
		s.rows[uid] = api.PodRow{
			UID:       uid,
			Name:      name,
			Namespace: "kube-system",
			Phase:     "Running",
			Ready:     "1/1",
			Restarts:  0,
			Node:      "spinoza-node",
			CreatedAt: now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		}
	}
	go s.loop(ctx, interval)
	return s
}

func (s *stub) loop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick()
		}
	}
}

func (s *stub) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	uid := "stub-ephemeral"
	_, exists := s.rows[uid]
	var ev Event
	if exists {
		delete(s.rows, uid)
		ev = Event{Kind: "deleted", UID: uid}
	} else {
		s.counter++
		row := api.PodRow{
			UID:       uid,
			Name:      fmt.Sprintf("ephemeral-job-%d", s.counter),
			Namespace: "default",
			Phase:     "Running",
			Ready:     "1/1",
			Restarts:  0,
			Node:      "spinoza-node",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		s.rows[uid] = row
		ev = Event{Kind: "added", Row: row}
	}
	s.rv++
	for ch := range s.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (s *stub) Snapshot() ([]api.PodRow, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([]api.PodRow, 0, len(s.rows))
	for _, r := range s.rows {
		rows = append(rows, r)
	}
	return rows, fmt.Sprintf("%d", s.rv)
}

func (s *stub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
	}
	return ch, cancel
}
