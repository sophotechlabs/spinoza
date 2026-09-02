package resources

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/discovery"
)

func TestUnsyncedStreamsAreNotReportedAsCached(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{},
		&unstructured.Unstructured{},
		0,
		cache.Indexers{},
	)
	mgr := &Manager{
		streams: map[streamKey]*stream{
			{gvr: gvr}: {kind: "Deployment", gvr: gvr, informer: informer},
		},
	}

	if got := mgr.syncedTypes(); len(got) != 0 {
		t.Fatalf("synced types = %v, want none before the informer has synced", got)
	}
}

func TestCachedDropsAStreamWhoseDescriptorWasRemoved(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	key := discovery.Key("apps", "v1", "deployments")
	desc := testDescs()[key]
	if _, err := mgr.List(t.Context(), desc); err != nil {
		t.Fatalf("List: %v", err)
	}

	mgr.catalog.Lock()
	delete(mgr.descs, key)
	mgr.catalog.Unlock()

	if got := mgr.Cached(); len(got) != 0 {
		t.Fatalf("cached = %+v, want the removed resource hidden", got)
	}
}
