package resources

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/discovery"
)

func workload(kind string, spec, status map[string]any) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       kind,
		"metadata":   map[string]any{"name": "web", "namespace": "default", "uid": "uid-web"},
	}
	if spec != nil {
		obj["spec"] = spec
	}
	if status != nil {
		obj["status"] = status
	}
	return &unstructured.Unstructured{Object: obj}
}

func conditioned(kind string, conditions ...map[string]any) *unstructured.Unstructured {
	entries := make([]any, 0, len(conditions))
	for _, condition := range conditions {
		entries = append(entries, condition)
	}
	return workload(kind, nil, map[string]any{"conditions": entries})
}

func TestUnhealthy(t *testing.T) {
	cases := []struct {
		name string
		obj  *unstructured.Unstructured
		want bool
	}{
		{
			"a deployment short of replicas",
			workload("Deployment", map[string]any{"replicas": int64(3)}, map[string]any{"readyReplicas": int64(1)}),
			true,
		},
		{
			"a deployment at full strength",
			workload("Deployment", map[string]any{"replicas": int64(3)}, map[string]any{"readyReplicas": int64(3)}),
			false,
		},
		{
			"a deployment scaled to zero",
			workload("Deployment", map[string]any{"replicas": int64(0)}, map[string]any{}),
			false,
		},
		{
			"a statefulset short of replicas",
			workload("StatefulSet", map[string]any{"replicas": int64(2)}, map[string]any{"readyReplicas": int64(0)}),
			true,
		},
		{
			"an old replicaset kept at zero",
			workload("ReplicaSet", map[string]any{"replicas": int64(0)}, map[string]any{}),
			false,
		},
		{
			"a daemonset missing nodes",
			workload("DaemonSet", nil, map[string]any{"desiredNumberScheduled": int64(5), "numberReady": int64(3)}),
			true,
		},
		{
			"a daemonset on every node",
			workload("DaemonSet", nil, map[string]any{"desiredNumberScheduled": int64(5), "numberReady": int64(5)}),
			false,
		},
		{
			"a failed job",
			conditioned("Job", map[string]any{"type": "Failed", "status": "True"}),
			true,
		},
		{
			"a completed job",
			conditioned("Job", map[string]any{"type": "Complete", "status": "True"}),
			false,
		},
		{
			"a custom resource that is not ready",
			conditioned("Kustomization", map[string]any{"type": "Ready", "status": "False", "reason": "BuildFailed"}),
			true,
		},
		{
			"a custom resource that is ready",
			conditioned("Kustomization", map[string]any{"type": "Ready", "status": "True"}),
			false,
		},
		{
			"an object with no readiness signal",
			workload("ConfigMap", nil, nil),
			false,
		},
		{
			"a pod is left to the server-side tally",
			conditioned("Pod", map[string]any{"type": "Ready", "status": "False"}),
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unhealthy(tc.obj, tc.obj.GetKind()); got != tc.want {
				t.Fatalf("unhealthy = %v, want %v", got, tc.want)
			}
		})
	}
}

func brokenDeployment(namespace, name string) *unstructured.Unstructured {
	broken := newDeployment(namespace, name)
	broken.Object["status"] = map[string]any{"readyReplicas": int64(0)}
	return broken
}

func TestCountsReportWatchedTypesThatAreFailing(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web"), brokenDeployment("default", "db")))
	defer cancel()

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "", 0, nil)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()

	got := mgr.Counts(context.Background())

	if got.Failing["apps/v1/deployments"] != 1 {
		t.Fatalf("failing = %v, want the one short deployment", got.Failing)
	}
}

func TestANamespacedViewStillTalliesTheWholeCluster(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, brokenDeployment("other", "db")))
	defer cancel()

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default", 0, nil)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()

	got := mgr.Counts(context.Background())

	if got.Failing["apps/v1/deployments"] != 1 {
		t.Fatalf("failing = %v, want the whole cluster counted behind a namespaced view", got.Failing)
	}
}

func TestCountsWithoutWatchedTypesLeaveFailingAlone(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, brokenDeployment("default", "db")))
	defer cancel()

	got := mgr.Counts(context.Background())

	if got.Failing != nil {
		t.Fatalf("failing = %v, want none without a watched type", got.Failing)
	}
}

func TestALeasedReadLetsTheStreamGoWhenNobodyIsWatching(t *testing.T) {
	mgr, cancel := newManagerWithGrace(t, newClient(t, newDeployment("default", "web")), time.Millisecond)
	defer cancel()
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	found, err := mgr.Lease(t.Context(), desc)
	if err != nil {
		t.Fatalf("Lease: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("found = %d objects, want the deployment", len(found))
	}

	waitForStreams(t, mgr, 0)
}

func TestAPinnedReadKeepsTheStreamOpen(t *testing.T) {
	mgr, cancel := newManagerWithGrace(t, newClient(t, newDeployment("default", "web")), time.Millisecond)
	defer cancel()
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	if _, err := mgr.List(t.Context(), desc); err != nil {
		t.Fatalf("List: %v", err)
	}

	waitForStreams(t, mgr, 1)
}

func TestASecondLeaseReusesTheStreamTheFirstOpened(t *testing.T) {
	mgr, cancel := newManagerWithGrace(t, newClient(t, newDeployment("default", "web")), time.Minute)
	defer cancel()
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	if _, err := mgr.Lease(t.Context(), desc); err != nil {
		t.Fatalf("first Lease: %v", err)
	}
	if _, err := mgr.Lease(t.Context(), desc); err != nil {
		t.Fatalf("second Lease: %v", err)
	}

	waitForStreams(t, mgr, 1)
}

func TestALeaseSaysWhenTheApiserverRefuses(t *testing.T) {
	dyn := newClient(t)
	dyn.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("deployments is forbidden")
	})
	ctx := t.Context()
	mgr := NewManager(ctx, Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Descriptors: testDescs(),
		Limits:      Limits{SyncTimeout: 50 * time.Millisecond, IdleGrace: time.Millisecond},
	})
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	_, err := mgr.Lease(ctx, desc)

	if err == nil {
		t.Fatal("Lease = nil error, want the refusal carried back")
	}
	if !strings.Contains(err.Error(), "deployments is forbidden") {
		t.Fatalf("error = %q, want the apiserver's own words", err.Error())
	}
	waitForStreams(t, mgr, 0)
}

func TestALeaseOfAnEmptyKindComesBackEmpty(t *testing.T) {
	mgr, cancel := newManagerWithGrace(t, newClient(t), time.Millisecond)
	defer cancel()
	desc := testDescs()[discovery.Key("apps", "v1", "deployments")]

	found, err := mgr.Lease(t.Context(), desc)
	if err != nil {
		t.Fatalf("Lease: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %d objects, want none", len(found))
	}
}
