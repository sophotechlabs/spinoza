package server

import "testing"

func TestLateHealthForAClosedClusterIsNotRemembered(t *testing.T) {
	srv, held := twoClusters(t, &pinger{}, &pinger{})
	generation := srv.healthGeneration(mk2)
	held.mu.Lock()
	delete(held.backends, mk2)
	held.mu.Unlock()

	srv.recordHealthAt(mk2, generation, notAnswering("late connection failure"))

	srv.mu.Lock()
	_, remembered := srv.health[mk2]
	srv.mu.Unlock()
	if remembered {
		t.Fatal("late health recreated state for a closed cluster")
	}
}
