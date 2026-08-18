package prom

import (
	"testing"
	"time"
)

func TestTheStepNeverGoesBelowItsFloor(t *testing.T) {
	if got := StepFor(time.Minute); got != minStep {
		t.Fatalf("step = %v, want the floor of %v for a short span", got, minStep)
	}
	if got := StepFor(24 * time.Hour); got <= minStep {
		t.Fatalf("step = %v, want a wider step for a day", got)
	}
}
