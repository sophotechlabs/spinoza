package resources

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestWithoutAPermissionServiceNothingIsClaimed(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	refs := []api.ObjectRef{{Version: "v1", Resource: "pods", Namespace: "default", Name: "web"}}

	bulk := mgr.AccessEach(t.Context(), "delete", refs)

	if len(bulk.Refused) != 0 {
		t.Fatalf("a manager with no permission service refused %d rows", len(bulk.Refused))
	}
}

func TestWhoCanDoWhatIsReadFromTheCluster(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	index := mgr.RBACIndex(t.Context())

	if index.Holders == nil && index.Error == "" {
		t.Fatal("the index came back with neither holders nor a reason")
	}
}
