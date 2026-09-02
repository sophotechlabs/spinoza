package resources

import (
	"errors"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

type refusingLister struct {
	err error
}

func (r refusingLister) List(labels.Selector) ([]runtime.Object, error) {
	return nil, r.err
}

func (r refusingLister) Get(string) (runtime.Object, error) {
	return nil, r.err
}

func (r refusingLister) ByNamespace(string) cache.GenericNamespaceLister {
	return r
}

func TestAFailedInitialSnapshotDetachesItsSubscriber(t *testing.T) {
	mgr := NewManager(t.Context(), Deps{
		Descriptors: testDescs(),
		Limits:      Limits{IdleGrace: time.Hour},
	})
	key := streamKey{gvr: depGVR}
	st := &stream{
		kind:   "Deployment",
		gvr:    depGVR,
		owner:  mgr,
		lister: refusingLister{err: errors.New("cache index is unavailable")},
		cancel: func() {},
		subs:   map[*subscriber]struct{}{},
	}
	mgr.streams[key] = st

	sub, err := mgr.Subscribe(t.Context(), "apps", "v1", "deployments", "shop", 0, nil)

	if sub != nil {
		t.Fatalf("subscription = %+v, want none without an initial snapshot", sub)
	}
	if err == nil || !strings.Contains(err.Error(), "reading the cached Deployment") {
		t.Fatalf("error = %v, want the cache failure identified", err)
	}
	st.mu.Lock()
	refs := st.refs
	remaining := len(st.subs)
	idle := st.idle
	st.mu.Unlock()
	if refs != 0 || remaining != 0 {
		t.Fatalf("refs = %d, subscribers = %d, want the failed subscriber detached", refs, remaining)
	}
	if idle == nil {
		t.Fatal("the failed stream was not scheduled for idle retirement")
	}
	idle.Stop()
}
