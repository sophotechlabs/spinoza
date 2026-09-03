package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const maxConsumedFlows = 4096

type consumedFlows struct {
	mu       sync.Mutex
	capacity int
	used     map[string]time.Time
	now      func() time.Time
}

func newConsumedFlows(capacity int, now func() time.Time) *consumedFlows {
	return &consumedFlows{capacity: capacity, used: map[string]time.Time{}, now: now}
}

func (f *consumedFlows) consume(state string, expires time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	for key, expiry := range f.used {
		if !now.Before(expiry) {
			delete(f.used, key)
		}
	}
	if _, found := f.used[state]; found {
		return errFlowReplay
	}
	if len(f.used) >= f.capacity {
		return errFlowRegistryFull
	}
	f.used[state] = expires
	return nil
}

type exchangeBudget struct {
	mu        sync.Mutex
	global    int
	perSource int
	used      int
	bySource  map[string]int
}

func newExchangeBudget(global, perSource int) *exchangeBudget {
	return &exchangeBudget{global: global, perSource: perSource, bySource: map[string]int{}}
}

func (b *exchangeBudget) claim(source string) (func(), bool) {
	b.mu.Lock()
	globalFull := b.used >= b.global
	sourceFull := b.bySource[source] >= b.perSource
	if globalFull || sourceFull {
		b.mu.Unlock()
		return nil, false
	}
	b.used++
	b.bySource[source]++
	b.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.used--
			b.bySource[source]--
			if b.bySource[source] == 0 {
				delete(b.bySource, source)
			}
			b.mu.Unlock()
		})
	}, true
}

func callbackSource(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}
