package resources

import (
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/openapi"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/portforward"
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
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Event",
		"metadata": map[string]any{
			"name":      "web.1",
			"namespace": "flux-system",
		},
		"involvedObject": map[string]any{"uid": "uid-web"},
		"reason":         "ScalingReplicaSet",
		"lastTimestamp":  "2026-07-27T09:30:00Z",
	}}
}

func inspectManager(t *testing.T, objs ...runtime.Object) *Manager {
	t.Helper()
	dyn := newClient(t, objs...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return NewManager(ctx, dyn, k8sfake.NewClientset(), nil, nil, nil, nil, nil, nil, testDescs())
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

	events, eventsErr := mgr.Events(context.Background(), "flux-system", "uid-web")
	if eventsErr != nil {
		t.Fatalf("events: %v", eventsErr)
	}

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

type stubGroupVersion struct{ doc string }

func (s stubGroupVersion) Schema(string) ([]byte, error) { return []byte(s.doc), nil }

func (s stubGroupVersion) ServerRelativeURL() string { return "" }

type stubOpenAPI struct {
	paths map[string]openapi.GroupVersion
}

func (s *stubOpenAPI) Paths() (map[string]openapi.GroupVersion, error) { return s.paths, nil }

const podSchemaDoc = `{"components":{"schemas":{
  "io.k8s.api.core.v1.Pod": {"x-kubernetes-group-version-kind": [{"group":"","kind":"Pod","version":"v1"}]}
}}}`

func TestManagerSchema(t *testing.T) {
	oapi := &stubOpenAPI{
		paths: map[string]openapi.GroupVersion{"api/v1": stubGroupVersion{doc: podSchemaDoc}},
	}
	schemas := jsonschema.NewClient(func() openapi.Client {
		return oapi
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := NewManager(ctx, newClient(t), k8sfake.NewClientset(), schemas, nil, nil, nil, nil, nil, testDescs())

	raw, err := mgr.Schema(jsonschema.GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	if !strings.Contains(string(raw), "io.k8s.api.core.v1.Pod") {
		t.Fatalf("bundle = %s", raw)
	}
}

func TestManagerSchemaWithoutASource(t *testing.T) {
	mgr := inspectManager(t)

	_, err := mgr.Schema(jsonschema.GVK{Version: "v1", Kind: "Pod"})

	if err == nil {
		t.Fatalf("expected an error when no schema source is configured")
	}
}

func TestManagerFluxAction(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	kustomization := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": "apps", "namespace": "flux-system"},
		"spec":       map[string]any{"suspend": false},
	}}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "KustomizationList"},
		kustomization,
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := NewManager(ctx, dyn, k8sfake.NewClientset(), nil, nil, nil, nil, nil, nil, testDescs())
	ref := api.ObjectRef{
		Group:     "kustomize.toolkit.fluxcd.io",
		Version:   "v1",
		Resource:  "kustomizations",
		Namespace: "flux-system",
		Name:      "apps",
	}

	_, err := mgr.FluxAction(context.Background(), ref, flux.Suspend)
	if err != nil {
		t.Fatalf("flux action: %v", err)
	}
	got, getErr := dyn.Resource(gvr).Namespace("flux-system").Get(context.Background(), "apps", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("read back: %v", getErr)
	}
	suspended, _, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if !suspended {
		t.Fatalf("spec.suspend was not set")
	}
}

type stubForwardRunner struct{ local int32 }

func (s *stubForwardRunner) Run(_ context.Context, _, _ string, _ int32, ready chan<- int32, stop <-chan struct{}) error {
	ready <- s.local
	<-stop
	return nil
}

type stubForwardResolver struct{}

func (stubForwardResolver) Resolve(_ context.Context, target portforward.Target, port int32) (string, int32, error) {
	return target.Name, port, nil
}

func forwardManager(t *testing.T) *Manager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := portforward.NewRegistry(ctx, &stubForwardRunner{local: 45123}, stubForwardResolver{}, nil)
	t.Cleanup(registry.StopAll)
	return NewManager(ctx, newClient(t), k8sfake.NewClientset(), nil, registry, nil, nil, nil, nil, testDescs())
}

func TestManagerPortForwardLifecycle(t *testing.T) {
	mgr := forwardManager(t)
	target := portforward.Target{Kind: portforward.KindPod, Namespace: "flux-system", Name: "web"}

	forward, err := mgr.StartForward(context.Background(), target, 8080)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if forward.LocalPort != 45123 {
		t.Fatalf("localPort = %d", forward.LocalPort)
	}
	if len(mgr.Forwards()) != 1 {
		t.Fatalf("forwards = %v", mgr.Forwards())
	}

	if err := mgr.StopForward(forward.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if len(mgr.Forwards()) != 0 {
		t.Fatalf("forward survived the stop")
	}
}

func TestManagerPortForwardWithoutARegistry(t *testing.T) {
	mgr := inspectManager(t)
	target := portforward.Target{Kind: portforward.KindPod, Namespace: "flux-system", Name: "web"}

	if _, err := mgr.StartForward(context.Background(), target, 8080); err == nil {
		t.Fatalf("expected an error when port forwarding is unavailable")
	}
	if len(mgr.Forwards()) != 0 {
		t.Fatalf("expected an empty list")
	}
	if err := mgr.StopForward("pf-1"); err == nil {
		t.Fatalf("expected an error when port forwarding is unavailable")
	}
}

func TestApplyRejectsADocumentForAnotherKindInTheSameGroup(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	t.Cleanup(cancel)
	ref := api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "default",
		Name:      "web",
	}
	doc := []byte("apiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: web\n  namespace: default\n")

	_, err := mgr.ApplyObject(context.Background(), ref, doc)

	if err == nil {
		t.Fatal("a StatefulSet document was accepted at the deployments endpoint")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Fatalf("err = %v, want it to name the kind mismatch", err)
	}
}

func TestApplyAllowsAnUnknownResourceThrough(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	t.Cleanup(cancel)
	ref := api.ObjectRef{Group: "x.example.com", Version: "v1", Resource: "widgets", Name: "one"}
	doc := []byte("apiVersion: x.example.com/v1\nkind: Widget\nmetadata:\n  name: one\n")

	_, err := mgr.ApplyObject(context.Background(), ref, doc)

	if err != nil && strings.Contains(err.Error(), "kind") {
		t.Fatalf("a resource missing from discovery was blocked on kind: %v", err)
	}
}
