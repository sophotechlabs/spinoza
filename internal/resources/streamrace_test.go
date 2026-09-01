package resources

import (
	"sync"
	"testing"
)

func streamWithSubs(count int) *stream {
	st := &stream{
		kind:   "Deployment",
		subs:   map[*subscriber]struct{}{},
		cancel: func() {},
		broken: true,
	}
	for range count {
		st.subs[newSubscriber("", 0, everything())] = struct{}{}
	}
	return st
}

func TestARecoveredWatchDoesNotSignalASubscriptionThatIsBeingClosed(t *testing.T) {
	for range 200 {
		st := streamWithSubs(8)
		var work sync.WaitGroup
		work.Go(st.watchRecovered)
		work.Go(st.shutdown)
		work.Wait()
	}
}

func TestARecoveredWatchTellsEverySubscriptionToResync(t *testing.T) {
	st := streamWithSubs(3)
	held := make([]*subscriber, 0, len(st.subs))
	for sub := range st.subs {
		held = append(held, sub)
	}

	st.watchRecovered()

	for at, sub := range held {
		select {
		case <-sub.resync:
		default:
			t.Fatalf("subscription %d was not told to resync after the watch came back", at)
		}
	}
}

func TestAWatchThatNeverBrokeSignalsNobody(t *testing.T) {
	st := streamWithSubs(3)
	st.broken = false
	held := make([]*subscriber, 0, len(st.subs))
	for sub := range st.subs {
		held = append(held, sub)
	}

	st.watchRecovered()

	for at, sub := range held {
		select {
		case <-sub.resync:
			t.Fatalf("subscription %d was told to resync although the watch never broke", at)
		default:
		}
	}
}

func TestABrokenWatchIsAnnouncedOnce(t *testing.T) {
	st := streamWithSubs(1)
	st.broken = false
	sub := newSubscriber("", 0, everything())
	st.subs[sub] = struct{}{}

	st.watchBroke("connection reset")
	st.watchBroke("connection reset")

	seen := 0
	for {
		select {
		case <-sub.events:
			seen++
			continue
		default:
		}
		break
	}
	if seen != 1 {
		t.Fatalf("the broken watch was announced %d times, want once", seen)
	}
}
