package resources

import (
	"errors"
	"testing"

	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

func TestHelmHistorySaysWhenHelmIsNotWiredUp(t *testing.T) {
	mgr := viewManager(t, nil)

	_, err := mgr.HelmHistory(t.Context(), "demo", "podinfo", 0)

	if !errors.Is(err, api.ErrInternal) {
		t.Fatalf("history error = %v, want an internal wiring failure", err)
	}
}

func TestHelmHistoryReachesTheHelmService(t *testing.T) {
	client := k8sfake.NewClientset()
	releases := helm.NewService(client, helmMeta(t, client), nil, nil, nil, api.ContextRef{})
	mgr := viewManager(t, releases)

	_, err := mgr.HelmHistory(t.Context(), "demo", "podinfo", 0)

	if !errors.Is(err, helm.ErrNoRelease) {
		t.Fatalf("history error = %v, want the empty helm store", err)
	}
}
