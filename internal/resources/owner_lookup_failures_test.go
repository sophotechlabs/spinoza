package resources

import (
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

func TestAConsumerListFailureDoesNotHideTheSource(t *testing.T) {
	mgr := sourceManager(t, gitRepository())
	mgr.syncTimeout = 10 * time.Millisecond
	dyn, ok := mgr.dyn.(interface {
		PrependReactor(string, string, k8stesting.ReactionFunc)
	})
	if !ok {
		t.Fatalf("dynamic client = %T, want a reactor", mgr.dyn)
	}
	dyn.PrependReactor("list", "kustomizations", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("kustomizations are forbidden")
	})

	detail, err := mgr.Object(t.Context(), sourceRef())
	if err != nil {
		t.Fatalf("object: %v", err)
	}
	if len(detail.Consumers) != 0 {
		t.Fatalf("consumers = %+v, want none from the failed lookup", detail.Consumers)
	}
}

func TestAnOwnerListFailureDoesNotLeaveAnUnresolvedOwner(t *testing.T) {
	mgr := argoManager(t, trackedDeployment())
	mgr.syncTimeout = 10 * time.Millisecond
	dyn, ok := mgr.dyn.(interface {
		PrependReactor(string, string, k8stesting.ReactionFunc)
	})
	if !ok {
		t.Fatalf("dynamic client = %T, want a reactor", mgr.dyn)
	}
	dyn.PrependReactor("list", "applications", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("applications are forbidden")
	})

	detail, err := mgr.Object(t.Context(), deploymentRef())
	if err != nil {
		t.Fatalf("object: %v", err)
	}
	if detail.ManagedBy != nil {
		t.Fatalf("owner = %+v, want a failed lookup cleared", detail.ManagedBy)
	}
}
