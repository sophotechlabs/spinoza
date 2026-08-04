package server

import (
	"context"
	"testing"
	"time"
)

func frozen(at *time.Time) func() time.Time {
	return func() time.Time {
		return *at
	}
}

func TestTheFirstResyncGoesStraightOut(t *testing.T) {
	at := time.Now()
	spacing := &throttle{interval: time.Hour, now: frozen(&at)}

	start := time.Now()
	if !spacing.wait(context.Background()) {
		t.Fatal("the first resync was refused")
	}

	if time.Since(start) > time.Second {
		t.Fatalf("the first resync waited %s", time.Since(start))
	}
}

func TestASecondResyncWaitsOutTheInterval(t *testing.T) {
	at := time.Now()
	spacing := &throttle{interval: 100 * time.Millisecond, now: frozen(&at)}
	if !spacing.wait(context.Background()) {
		t.Fatal("the first resync was refused")
	}

	start := time.Now()
	if !spacing.wait(context.Background()) {
		t.Fatal("the second resync was refused")
	}
	waited := time.Since(start)

	if waited < 50*time.Millisecond {
		t.Fatalf("the second resync waited %s, want it spaced out from the first", waited)
	}
}

func TestAResyncStopsWaitingWhenTheSessionEnds(t *testing.T) {
	at := time.Now()
	spacing := &throttle{interval: time.Hour, now: frozen(&at)}
	spacing.wait(context.Background())
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	allowed := spacing.wait(ctx)

	if allowed {
		t.Fatal("a resync was sent after the session ended")
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("the wait took %s, want it cut short with the session", time.Since(start))
	}
}

func TestTimePassingLetsTheNextResyncThrough(t *testing.T) {
	at := time.Now()
	spacing := &throttle{interval: time.Second, now: frozen(&at)}
	spacing.wait(context.Background())

	at = at.Add(2 * time.Second)
	start := time.Now()
	if !spacing.wait(context.Background()) {
		t.Fatal("the resync was refused")
	}

	if time.Since(start) > time.Second {
		t.Fatalf("waited %s, want no wait once the interval had passed", time.Since(start))
	}
}
