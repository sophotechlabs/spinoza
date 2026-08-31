package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

var cronJobGVR = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}

const cronQuery = "?group=batch&version=v1&resource=cronjobs&namespace=shop&name=nightly"

func replicasOf(t *testing.T, dyn dynamic.Interface) int64 {
	t.Helper()
	stored, err := dyn.Resource(actionDeploymentGVR).Namespace("shop").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	replicas, _, _ := unstructured.NestedInt64(stored.Object, "spec", "replicas")
	return replicas
}

func cronJob() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata":   map[string]any{"name": "nightly", "namespace": "shop"},
		"spec":       map[string]any{"schedule": "0 2 * * *", "suspend": false},
	}}
}

func cronActionServer(t *testing.T) (*httptest.Server, dynamic.Interface) {
	t.Helper()
	kinds := map[schema.GroupVersionResource]string{cronJobGVR: "CronJobList"}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, cronJob())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("batch", "v1", "cronjobs"): {
			Group: "batch", Version: "v1", Resource: "cronjobs", Kind: "CronJob", Namespaced: true,
		},
	}
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})
	ts := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	return ts, dyn
}

func TestCronJobActionsOnAProtectedClusterNeedTheNameTyped(t *testing.T) {
	for _, action := range []string{"suspend", "resume", "trigger"} {
		t.Run(action, func(t *testing.T) {
			ts, dyn := cronActionServer(t)
			protect(t, ts)

			resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/action"+cronQuery+"&action="+action, nil)

			if resp.StatusCode != http.StatusPreconditionFailed {
				t.Fatalf("status = %d, want 412 for %s", resp.StatusCode, action)
			}
			if suspendedOf(t, dyn) {
				t.Fatalf("%s changed the cronjob anyway", action)
			}
		})
	}
}

func TestSuspendingACronJobGoesAheadOnceTheNameMatches(t *testing.T) {
	ts, dyn := cronActionServer(t)
	protect(t, ts)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+cronQuery+"&action=suspend&confirm=nightly", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the confirmed suspend to go through: %s", resp.StatusCode, body)
	}
	if !suspendedOf(t, dyn) {
		t.Fatal("the cronjob was not suspended")
	}
}

func suspendedOf(t *testing.T, dyn dynamic.Interface) bool {
	t.Helper()
	stored, err := dyn.Resource(cronJobGVR).Namespace("shop").Get(context.Background(), "nightly", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	suspended, _, _ := unstructured.NestedBool(stored.Object, "spec", "suspend")
	return suspended
}
