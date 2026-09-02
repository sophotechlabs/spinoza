package server

import (
	"testing"
	"time"
)

func TestRepeatedIdleChecksKeepOnePendingExitTimer(t *testing.T) {
	state := views{armed: true, grace: time.Hour}
	state.reconsider()
	pending := state.timer
	if pending == nil {
		t.Fatal("an armed idle view scheduled no exit timer")
	}

	state.reconsider()

	if state.timer != pending {
		t.Fatal("a repeated idle check replaced the pending exit timer")
	}
	pending.Stop()
}

func TestShowingAWindowCancelsThePendingIdleExit(t *testing.T) {
	state := views{
		armed:   true,
		desktop: 1,
		hidden:  true,
		grace:   time.Hour,
	}
	state.reconsider()
	if state.timer == nil {
		t.Fatal("the hidden desktop scheduled no idle exit")
	}

	state.show()

	if state.timer != nil {
		state.timer.Stop()
		t.Fatal("showing the desktop left its idle exit pending")
	}
}
