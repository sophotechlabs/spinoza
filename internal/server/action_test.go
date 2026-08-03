package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
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

var (
	actionDeploymentGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	actionNodeGVR       = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
)

const (
	deploymentQuery = "?group=apps&version=v1&resource=deployments&namespace=shop&name=web"
	nodeQuery       = "?version=v1&resource=nodes&name=worker-1"
)

func actionDeployment() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "web", "namespace": "shop"},
		"spec":       map[string]any{"replicas": int64(1)},
	}}
}

func actionNode() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "worker-1"},
		"spec":       map[string]any{},
	}}
}

func actionServer(t *testing.T, pods []runtime.Object, objs ...runtime.Object) (*httptest.Server, dynamic.Interface) {
	t.Helper()
	kinds := map[schema.GroupVersionResource]string{
		actionDeploymentGVR: "DeploymentList",
		actionNodeGVR:       "NodeList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("apps", "v1", "deployments"): {
			Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true,
		},
	}
	mgr := resources.NewManager(ctx, dyn, k8sfake.NewClientset(pods...), nil, nil, nil, nil, nil, nil, descs)
	ts := httptest.NewServer(New(fixed(mgr), testAssets()).Handler())
	t.Cleanup(ts.Close)
	return ts, dyn
}

func decodeResult(t *testing.T, body []byte) api.ActionResult {
	t.Helper()
	var result api.ActionResult
	err := json.Unmarshal(body, &result)
	if err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return result
}

func nodePod(name string) *corev1.Pod {
	yes := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "shop",
			Name:      name,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       "web",
				Controller: &yes,
			}},
		},
		Spec:   corev1.PodSpec{NodeName: "worker-1"},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func TestActionScalesADeployment(t *testing.T) {
	ts, dyn := actionServer(t, nil, actionDeployment())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+deploymentQuery+"&action=scale&replicas=4", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	stored, err := dyn.Resource(actionDeploymentGVR).Namespace("shop").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	replicas, _, _ := unstructured.NestedInt64(stored.Object, "spec", "replicas")
	if replicas != 4 {
		t.Fatalf("spec.replicas = %d, want 4", replicas)
	}
	if decodeResult(t, body).Action != "scale" {
		t.Fatalf("body = %s", body)
	}
}

func TestActionRestartsADeployment(t *testing.T) {
	ts, dyn := actionServer(t, nil, actionDeployment())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+deploymentQuery+"&action=restart", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	stored, _ := dyn.Resource(actionDeploymentGVR).Namespace("shop").Get(context.Background(), "web", metav1.GetOptions{})
	annotations, _, _ := unstructured.NestedStringMap(stored.Object, "spec", "template", "metadata", "annotations")
	if annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatalf("restart annotation missing: %v", annotations)
	}
}

func TestActionCordonsANode(t *testing.T) {
	ts, dyn := actionServer(t, nil, actionNode())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+nodeQuery+"&action=cordon", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	stored, _ := dyn.Resource(actionNodeGVR).Get(context.Background(), "worker-1", metav1.GetOptions{})
	unschedulable, _, _ := unstructured.NestedBool(stored.Object, "spec", "unschedulable")
	if !unschedulable {
		t.Fatal("node was not cordoned")
	}
}

func TestActionPlansADrain(t *testing.T) {
	ts, _ := actionServer(t, []runtime.Object{nodePod("web-1")}, actionNode())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+nodeQuery+"&action=drain&dryRun=true", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	result := decodeResult(t, body)
	if !result.DryRun {
		t.Fatalf("body = %s, want a dry run", body)
	}
	if len(result.Pods) != 1 || result.Pods[0].Outcome != api.OutcomeEvict {
		t.Fatalf("pods = %+v", result.Pods)
	}
}

func TestActionRejectsAGetRequest(t *testing.T) {
	ts, _ := actionServer(t, nil, actionDeployment())

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/action"+deploymentQuery+"&action=scale", nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestActionRejectsAMissingName(t *testing.T) {
	ts, _ := actionServer(t, nil, actionDeployment())

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/action?version=v1&resource=deployments&action=scale", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestActionRejectsANonNumericReplicaCount(t *testing.T) {
	ts, _ := actionServer(t, nil, actionDeployment())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+deploymentQuery+"&action=scale&replicas=many", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(string(body), "replicas") {
		t.Fatalf("body = %s", body)
	}
}

func TestActionRejectsAnUnsupportedPair(t *testing.T) {
	ts, _ := actionServer(t, nil, actionDeployment())

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/action"+deploymentQuery+"&action=drain", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestActionRefusesToScaleWithoutAReplicaCount(t *testing.T) {
	ts, dyn := actionServer(t, nil, actionDeployment())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+deploymentQuery+"&action=scale", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
	stored, _ := dyn.Resource(actionDeploymentGVR).Namespace("shop").Get(context.Background(), "web", metav1.GetOptions{})
	replicas, _, _ := unstructured.NestedInt64(stored.Object, "spec", "replicas")
	if replicas != 1 {
		t.Fatalf("spec.replicas = %d, want the deployment untouched", replicas)
	}
}

func TestActionScalesToZeroWhenAskedTo(t *testing.T) {
	ts, dyn := actionServer(t, nil, actionDeployment())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+deploymentQuery+"&action=scale&replicas=0", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	stored, _ := dyn.Resource(actionDeploymentGVR).Namespace("shop").Get(context.Background(), "web", metav1.GetOptions{})
	replicas, found, _ := unstructured.NestedInt64(stored.Object, "spec", "replicas")
	if !found || replicas != 0 {
		t.Fatalf("spec.replicas = %d (found %v), want 0", replicas, found)
	}
}
