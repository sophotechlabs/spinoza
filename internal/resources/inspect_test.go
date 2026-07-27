package resources

import (
	"context"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/logs"
)

func deploymentRef() api.ObjectRef {
	return api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "flux-system",
		Name:      "web",
	}
}

func newDeploymentEvent() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Event",
		"metadata": map[string]interface{}{
			"name":      "web.1",
			"namespace": "flux-system",
		},
		"involvedObject": map[string]interface{}{"uid": "uid-web"},
		"reason":         "ScalingReplicaSet",
		"lastTimestamp":  "2026-07-27T09:30:00Z",
	}}
}

func inspectManager(t *testing.T, objs ...runtime.Object) *Manager {
	t.Helper()
	dyn := newClient(t, objs...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return NewManager(ctx, dyn, k8sfake.NewClientset(), nil, nil, testDescs())
}

func TestManagerObject(t *testing.T) {
	mgr := inspectManager(t, newDeployment("flux-system", "web"))

	detail, err := mgr.Object(context.Background(), deploymentRef())
	if err != nil {
		t.Fatalf("object: %v", err)
	}
	if detail.Name != "web" {
		t.Fatalf("name = %q, want web", detail.Name)
	}
	if !strings.Contains(detail.YAML, "kind: Deployment") {
		t.Fatalf("yaml = %q", detail.YAML)
	}
}

func TestManagerApplyObject(t *testing.T) {
	mgr := inspectManager(t, newDeployment("flux-system", "web"))
	doc := []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: flux-system\n  labels:\n    app: edited\n")

	detail, err := mgr.ApplyObject(context.Background(), deploymentRef(), doc)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if detail.Labels["app"] != "edited" {
		t.Fatalf("labels = %v", detail.Labels)
	}
}

func TestManagerDeleteObject(t *testing.T) {
	mgr := inspectManager(t, newDeployment("flux-system", "web"))

	err := mgr.DeleteObject(context.Background(), deploymentRef())
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, getErr := mgr.Object(context.Background(), deploymentRef())
	if getErr == nil {
		t.Fatalf("object still readable after delete")
	}
}

func TestManagerEvents(t *testing.T) {
	mgr := inspectManager(t, newDeployment("flux-system", "web"), newDeploymentEvent())

	events := mgr.Events(context.Background(), "flux-system", "uid-web")

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Reason != "ScalingReplicaSet" {
		t.Fatalf("reason = %q", events[0].Reason)
	}
}

func TestManagerLogs(t *testing.T) {
	mgr := inspectManager(t)

	stream, err := mgr.Logs(context.Background(), logs.Request{Namespace: "flux-system", Name: "web"})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	defer stream.Close()

	select {
	case line, ok := <-stream.Lines:
		if !ok {
			t.Fatalf("log channel closed without a line")
		}
		if line == "" {
			t.Fatalf("empty log line")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for a log line")
	}
}
