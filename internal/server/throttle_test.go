package server

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func frozen(at *time.Time) func() time.Time {
	return func() time.Time {
		return *at
	}
}

func TestTheFirstResyncGoesStraightOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		at := time.Now()
		spacing := &throttle{interval: time.Hour, now: frozen(&at)}

		start := time.Now()
		if !spacing.wait(context.Background()) {
			t.Fatal("the first resync was refused")
		}

		if waited := time.Since(start); waited != 0 {
			t.Fatalf("the first resync waited %s", waited)
		}
	})
}

func TestASecondResyncWaitsOutTheInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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

		if waited != 100*time.Millisecond {
			t.Fatalf("the second resync waited %s", waited)
		}
	})
}

func TestAResyncStopsWaitingWhenTheSessionEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		spacing := newThrottle(time.Hour)
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
		if waited := time.Since(start); waited != 50*time.Millisecond {
			t.Fatalf("the canceled wait took %s", waited)
		}
	})
}

func TestACancelledResyncDoesNotResetTheInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		spacing := newThrottle(100 * time.Millisecond)
		spacing.wait(context.Background())
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(40 * time.Millisecond)
			cancel()
		}()
		if spacing.wait(ctx) {
			t.Fatal("the canceled resync was allowed")
		}

		start := time.Now()
		if !spacing.wait(context.Background()) {
			t.Fatal("the next resync was refused")
		}
		if waited := time.Since(start); waited != 60*time.Millisecond {
			t.Fatalf("the next resync waited %s", waited)
		}
	})
}

func TestAResyncDoesNotWaitOnAnEndedSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		at := time.Now()
		spacing := &throttle{interval: time.Hour, now: frozen(&at)}
		spacing.wait(context.Background())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		start := time.Now()
		allowed := spacing.wait(ctx)

		if allowed {
			t.Fatal("a resync was sent on an ended session")
		}
		if waited := time.Since(start); waited != 0 {
			t.Fatalf("the ended session waited %s", waited)
		}
	})
}

func TestTimePassingLetsTheNextResyncThrough(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		at := time.Now()
		spacing := &throttle{interval: time.Second, now: frozen(&at)}
		spacing.wait(context.Background())

		at = at.Add(time.Second)
		start := time.Now()
		if !spacing.wait(context.Background()) {
			t.Fatal("the resync was refused")
		}

		if waited := time.Since(start); waited != 0 {
			t.Fatalf("waited %s at the interval boundary", waited)
		}
	})
}
