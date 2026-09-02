package resources

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func TestRetirementDoesNotCancelAStreamThatBecameBusy(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	key := streamKey{gvr: gvr}
	canceled := false
	st := &stream{gvr: gvr, refs: 1, cancel: func() { canceled = true }}
	mgr := &Manager{streams: map[streamKey]*stream{key: st}}

	mgr.retire(key, st)

	if canceled {
		t.Fatal("a stream with a new subscriber was canceled by its old idle timer")
	}
	if got := mgr.streams[key]; got != st {
		t.Fatalf("stream = %p, want the busy stream %p retained", got, st)
	}
}

func TestPinRejectsAStreamThatWasReplaced(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	key := streamKey{gvr: gvr}
	current := &stream{gvr: gvr}
	stale := &stream{gvr: gvr}
	mgr := &Manager{streams: map[streamKey]*stream{key: current}}

	if mgr.pin(key, stale) {
		t.Fatal("a replaced stream was pinned back into service")
	}
	if stale.pinned || current.pinned {
		t.Fatalf("stale pinned = %v, current pinned = %v, want neither changed", stale.pinned, current.pinned)
	}
}

func TestMalformedInformerEventsAreIgnored(t *testing.T) {
	sub := newSubscriber("", 0, everything())
	st := &stream{
		kind: "Deployment",
		subs: map[*subscriber]struct{}{sub: {}},
	}

	st.publish("added", "not a kubernetes object")
	st.publishDelete(cache.DeletedFinalStateUnknown{Obj: "not a kubernetes object"})

	select {
	case event := <-sub.events:
		t.Fatalf("malformed event was delivered: %+v", event)
	default:
	}
	select {
	case <-sub.resync:
		t.Fatal("malformed event requested a resync")
	default:
	}
}
