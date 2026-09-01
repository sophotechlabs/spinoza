package server

import (
	"sync"
	"testing"
)

func TestConcurrentHealthWritersLeaveAConsistentVerdict(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{})

	for range 1000 {
		srv.forgetHealthOf(mk1)
		start := make(chan struct{})
		var writers sync.WaitGroup
		for writer := range 16 {
			writers.Go(func() {
				<-start
				if writer%3 == 0 {
					srv.recordHealthOf(mk1, answering())
					return
				}
				srv.recordHealthOf(mk1, notAnswering("connection refused"))
			})
		}
		close(start)
		writers.Wait()

		srv.mu.Lock()
		misses := srv.misses[mk1]
		health := srv.health[mk1]
		srv.mu.Unlock()

		if misses == 0 {
			if !health.Reachable || health.Wobbling {
				t.Fatalf("misses = %d health = %+v", misses, health)
			}
			continue
		}
		if misses < missesBeforeUnreachable {
			if !health.Reachable || !health.Wobbling {
				t.Fatalf("misses = %d health = %+v", misses, health)
			}
			continue
		}
		if health.Reachable || health.Wobbling {
			t.Fatalf("misses = %d health = %+v", misses, health)
		}
	}
}
