package resources

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestAnUnansweredPermissionReviewRefusesTheRow(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	refs := []api.ObjectRef{{Version: "v1", Resource: "pods", Namespace: "default", Name: "web"}}

	bulk := mgr.AccessEach(t.Context(), "delete", refs)

	if len(bulk.Refused) != 1 {
		t.Fatalf("refused = %v, want the unanswered row kept unavailable", bulk.Refused)
	}
	if bulk.Refused[0].At != 0 || bulk.Refused[0].Reason == "" {
		t.Fatalf("refusal = %+v, want the first row and a reason", bulk.Refused[0])
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

func TestBulkAccessWithoutPermissionsIsEmpty(t *testing.T) {
	manager := NewManager(t.Context(), Deps{})
	refs := []api.ObjectRef{
		{Version: "v1", Resource: "pods", Namespace: "prod", Name: "web-0"},
		{Version: "v1", Resource: "pods", Namespace: "prod", Name: "api-0"},
	}

	access := manager.AccessEach(t.Context(), "restart", refs)

	if len(access.Refused) != 0 {
		t.Fatalf("refused = %v, want no invented decisions", access.Refused)
	}
}
