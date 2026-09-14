package auth

import (
	"context"
	"log/slog"
	"maps"
	"sync"
	"time"
)

type RevocationStore interface {
	Revoke(ctx context.Context, session string, until, now time.Time) error
	Revocations(ctx context.Context, now time.Time) (map[string]time.Time, error)
}

type revocations struct {
	mu     sync.Mutex
	gone   map[string]time.Time
	within time.Duration
	now    func() time.Time
	kept   RevocationStore
}

func newRevocations(within time.Duration) *revocations {
	return &revocations{gone: map[string]time.Time{}, within: within, now: time.Now}
}

func (rv *revocations) keep(ctx context.Context, kept RevocationStore) error {
	held, err := kept.Revocations(ctx, rv.now())
	if err != nil {
		return err
	}
	rv.mu.Lock()
	defer rv.mu.Unlock()
	rv.kept = kept
	maps.Copy(rv.gone, held)
	return nil
}

func (rv *revocations) revoke(ctx context.Context, session string) {
	if session == "" {
		return
	}
	rv.mu.Lock()
	rv.sweep()
	now := rv.now()
	until := now.Add(rv.within)
	rv.gone[session] = until
	kept := rv.kept
	rv.mu.Unlock()
	if kept == nil {
		return
	}
	err := kept.Revoke(ctx, session, until, now)
	if err != nil {
		slog.Warn("a revoked session could not be written down, so it would be accepted again after a restart", "session", session, "error", err)
	}
}

func (rv *revocations) revoked(session string) bool {
	if session == "" {
		return false
	}
	rv.mu.Lock()
	defer rv.mu.Unlock()
	until, held := rv.gone[session]
	if !held {
		return false
	}
	if !rv.now().Before(until) {
		delete(rv.gone, session)
		return false
	}
	return true
}

func (rv *revocations) sweep() {
	now := rv.now()
	for session, until := range rv.gone {
		if !now.Before(until) {
			delete(rv.gone, session)
		}
	}
}
