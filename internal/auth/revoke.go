package auth

import (
	"sync"
	"time"
)

type revocations struct {
	mu     sync.Mutex
	gone   map[string]time.Time
	within time.Duration
	now    func() time.Time
}

func newRevocations(within time.Duration) *revocations {
	return &revocations{gone: map[string]time.Time{}, within: within, now: time.Now}
}

func (rv *revocations) revoke(session string) {
	if session == "" {
		return
	}
	rv.mu.Lock()
	defer rv.mu.Unlock()
	rv.sweep()
	rv.gone[session] = rv.now().Add(rv.within)
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
