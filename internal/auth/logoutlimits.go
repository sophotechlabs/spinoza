package auth

import (
	"sync"
	"time"
)

const (
	logoutGlobalConcurrent = 4
	logoutSourceConcurrent = 2
	logoutGlobalRate       = 32
	logoutSourceRate       = 8
	logoutSourceCapacity   = 1024
	logoutRateWindow       = time.Minute
	logoutFailureCooldown  = 5 * time.Second
)

type logoutSourceWindow struct {
	events []time.Time
	last   time.Time
	used   int
}

type logoutVerificationBudget struct {
	mu               sync.Mutex
	globalConcurrent int
	sourceConcurrent int
	globalRate       int
	sourceRate       int
	sourceCapacity   int
	window           time.Duration
	cooldown         time.Duration
	used             int
	events           []time.Time
	sources          map[string]logoutSourceWindow
	failureUntil     time.Time
	now              func() time.Time
}

func newLogoutVerificationBudget(now func() time.Time) *logoutVerificationBudget {
	return &logoutVerificationBudget{
		globalConcurrent: logoutGlobalConcurrent,
		sourceConcurrent: logoutSourceConcurrent,
		globalRate:       logoutGlobalRate,
		sourceRate:       logoutSourceRate,
		sourceCapacity:   logoutSourceCapacity,
		window:           logoutRateWindow,
		cooldown:         logoutFailureCooldown,
		sources:          map[string]logoutSourceWindow{},
		now:              now,
	}
}

func (b *logoutVerificationBudget) claim(source string) (func(), bool) {
	b.mu.Lock()
	now := b.now()
	if now.Before(b.failureUntil) {
		b.mu.Unlock()
		return nil, false
	}
	b.events = recentLogoutEvents(b.events, now.Add(-b.window))
	held := b.sources[source]
	held.events = recentLogoutEvents(held.events, now.Add(-b.window))
	globalFull := b.used >= b.globalConcurrent
	sourceFull := held.used >= b.sourceConcurrent
	globalRateFull := len(b.events) >= b.globalRate
	sourceRateFull := len(held.events) >= b.sourceRate
	if globalFull || sourceFull || globalRateFull || sourceRateFull {
		b.mu.Unlock()
		return nil, false
	}
	if _, known := b.sources[source]; !known {
		if !b.evictLogoutSource() {
			b.mu.Unlock()
			return nil, false
		}
	}
	b.used++
	b.events = append(b.events, now)
	held.used++
	held.last = now
	held.events = append(held.events, now)
	b.sources[source] = held
	b.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.used--
			held := b.sources[source]
			held.used--
			b.sources[source] = held
			b.mu.Unlock()
		})
	}, true
}

func (b *logoutVerificationBudget) failed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failureUntil = b.now().Add(b.cooldown)
}

func (b *logoutVerificationBudget) evictLogoutSource() bool {
	if len(b.sources) < b.sourceCapacity {
		return true
	}
	oldest := ""
	var oldestAt time.Time
	for source, held := range b.sources {
		if held.used > 0 {
			continue
		}
		if oldest != "" && !held.last.Before(oldestAt) {
			continue
		}
		oldest = source
		oldestAt = held.last
	}
	if oldest != "" {
		delete(b.sources, oldest)
		return true
	}
	return false
}

func recentLogoutEvents(events []time.Time, cutoff time.Time) []time.Time {
	first := 0
	for first < len(events) {
		if events[first].After(cutoff) {
			break
		}
		first++
	}
	return events[first:]
}
