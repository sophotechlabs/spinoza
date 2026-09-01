package server

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type sizedSub struct {
	limit atomic.Int64
}

func (s *sizedSub) Close() {}

func (s *sizedSub) SetLimit(limit int) {
	s.limit.Store(int64(limit))
}

func newWSSession(t *testing.T) *wsSession {
	t.Helper()
	return &wsSession{
		ctx:    t.Context(),
		tables: map[string]*entry{},
		logs:   map[string]*entry{},
	}
}

func TestLoadMoreDoesNotReadASubscriptionWhileItIsBeingAdopted(t *testing.T) {
	for range 200 {
		sess := newWSSession(t)
		gen := sess.claim(tables, "sub-1")
		var work sync.WaitGroup
		work.Go(func() {
			sess.adopt(tables, "sub-1", gen, &sizedSub{})
		})
		work.Go(func() {
			sess.more(api.ClientMsg{SubID: "sub-1", Limit: 200})
		})
		work.Wait()
	}
}

func TestLoadMoreResizesTheSubscriptionOnceItIsAdopted(t *testing.T) {
	sess := newWSSession(t)
	gen := sess.claim(tables, "sub-1")
	sub := &sizedSub{}
	if !sess.adopt(tables, "sub-1", gen, sub) {
		t.Fatal("the subscription was not adopted")
	}

	sess.more(api.ClientMsg{SubID: "sub-1", Limit: 200})

	if sub.limit.Load() != 200 {
		t.Fatalf("limit = %d, want the 200 the client asked for", sub.limit.Load())
	}
}

func TestLoadMoreOnASubscriptionThatIsStillBuildingIsDropped(t *testing.T) {
	sess := newWSSession(t)
	sess.claim(tables, "sub-1")

	sess.more(api.ClientMsg{SubID: "sub-1", Limit: 200})
}

func TestLoadMoreOnAnUnknownSubscriptionIsDropped(t *testing.T) {
	sess := newWSSession(t)

	sess.more(api.ClientMsg{SubID: "nobody", Limit: 200})
}
