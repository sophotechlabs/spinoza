package server

import "testing"

func TestShellDrainWaitsForEveryOpenShell(t *testing.T) {
	var tally shellTally
	tally.start()
	tally.start()
	drained := tally.waiter()

	tally.done()

	select {
	case <-drained:
		t.Fatal("the drain finished while one shell was still open")
	default:
	}
	if tally.count() != 1 {
		t.Fatalf("open = %d, want 1", tally.count())
	}

	tally.done()

	select {
	case <-drained:
	default:
		t.Fatal("the drain stayed blocked after every shell closed")
	}
}

func TestAReopenedShellGetsANewDrainCycle(t *testing.T) {
	var tally shellTally
	tally.start()
	first := tally.waiter()
	tally.done()

	tally.start()
	second := tally.waiter()

	if first == second {
		t.Fatal("a reopened shell reused the completed drain signal")
	}
	select {
	case <-second:
		t.Fatal("the new drain cycle was already complete")
	default:
	}
	tally.done()
}
