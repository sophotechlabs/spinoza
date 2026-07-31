package inspect

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var (
	podGVR     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	nodeGVR    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	serviceGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
)

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		podGVR:     "PodList",
		nodeGVR:    "NodeList",
		serviceGVR: "ServiceList",
		eventGVR:   "EventList",
	}
}

func newClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), objs...)
}

func podRef() api.ObjectRef {
	return api.ObjectRef{Version: "v1", Resource: "pods", Namespace: "flux-system", Name: "web"}
}

func newPod() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":              "web",
			"namespace":         "flux-system",
			"uid":               "pod-uid",
			"creationTimestamp": "2026-07-27T09:00:00Z",
			"labels":            map[string]any{"app": "web"},
			"annotations": map[string]any{
				"note":                "keep",
				lastAppliedAnnotation: `{"spec":{}}`,
			},
			"ownerReferences": []any{
				map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "ReplicaSet",
					"name":       "web-abc",
					"uid":        "rs-uid",
				},
			},
			"managedFields": []any{
				map[string]any{"manager": "kubelet", "operation": "Update"},
			},
		},
		"spec": map[string]any{
			"initContainers": []any{
				map[string]any{"name": "init-db"},
			},
			"containers": []any{
				map[string]any{"name": "app"},
				map[string]any{"name": "sidecar"},
			},
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{
					"type":               "Ready",
					"status":             "True",
					"reason":             "PodReady",
					"message":            "all good",
					"lastTransitionTime": "2026-07-27T09:01:00Z",
				},
			},
		},
	}}
}

func TestGetReturnsDetail(t *testing.T) {
	detail, err := Get(context.Background(), newClient(newPod()), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Kind != "Pod" {
		t.Fatalf("kind = %q, want Pod", detail.Kind)
	}
	if detail.Name != "web" {
		t.Fatalf("name = %q, want web", detail.Name)
	}
	if detail.Namespace != "flux-system" {
		t.Fatalf("namespace = %q, want flux-system", detail.Namespace)
	}
	if detail.UID != "pod-uid" {
		t.Fatalf("uid = %q, want pod-uid", detail.UID)
	}
	if detail.CreatedAt != "2026-07-27T09:00:00Z" {
		t.Fatalf("createdAt = %q", detail.CreatedAt)
	}
	if detail.Labels["app"] != "web" {
		t.Fatalf("labels = %v", detail.Labels)
	}
	if len(detail.Owners) != 1 || detail.Owners[0].Name != "web-abc" {
		t.Fatalf("owners = %v", detail.Owners)
	}
	if len(detail.Conditions) != 1 {
		t.Fatalf("conditions = %v", detail.Conditions)
	}
	if detail.Conditions[0].Updated != "2026-07-27T09:01:00Z" {
		t.Fatalf("condition updated = %q", detail.Conditions[0].Updated)
	}
	want := []string{"init-db", "app", "sidecar"}
	if strings.Join(detail.Containers, ",") != strings.Join(want, ",") {
		t.Fatalf("containers = %v, want %v", detail.Containers, want)
	}
}

func TestGetStripsManagedFieldsAndLastApplied(t *testing.T) {
	detail, err := Get(context.Background(), newClient(newPod()), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.Contains(detail.YAML, "managedFields") {
		t.Fatalf("yaml still contains managedFields:\n%s", detail.YAML)
	}
	if strings.Contains(detail.YAML, lastAppliedAnnotation) {
		t.Fatalf("yaml still contains the last-applied annotation:\n%s", detail.YAML)
	}
	if !strings.Contains(detail.YAML, "note: keep") {
		t.Fatalf("yaml dropped the other annotations:\n%s", detail.YAML)
	}
	if detail.Annotations[lastAppliedAnnotation] != "" {
		t.Fatalf("annotations still carry last-applied: %v", detail.Annotations)
	}
	if detail.Annotations["note"] != "keep" {
		t.Fatalf("annotations = %v", detail.Annotations)
	}
}

func TestGetDropsEmptyAnnotationMap(t *testing.T) {
	pod := newPod()
	unstructured.RemoveNestedField(pod.Object, "metadata", "annotations")
	pod.SetAnnotations(map[string]string{lastAppliedAnnotation: "{}"})

	detail, err := Get(context.Background(), newClient(pod), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.Contains(detail.YAML, "annotations") {
		t.Fatalf("yaml kept an empty annotations map:\n%s", detail.YAML)
	}
}

func TestGetClusterScoped(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "p-mk1"},
	}}
	ref := api.ObjectRef{Version: "v1", Resource: "nodes", Name: "p-mk1"}

	detail, err := Get(context.Background(), newClient(node), ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Name != "p-mk1" {
		t.Fatalf("name = %q, want p-mk1", detail.Name)
	}
	if detail.CreatedAt != "" {
		t.Fatalf("createdAt = %q, want empty", detail.CreatedAt)
	}
	if detail.Owners != nil {
		t.Fatalf("owners = %v, want nil", detail.Owners)
	}
	if detail.Conditions != nil {
		t.Fatalf("conditions = %v, want nil", detail.Conditions)
	}
	if detail.Containers != nil {
		t.Fatalf("containers = %v, want nil", detail.Containers)
	}
}

func TestGetMissingObject(t *testing.T) {
	_, err := Get(context.Background(), newClient(), podRef())
	if err == nil {
		t.Fatalf("expected an error for a missing object")
	}
}

func TestApplyUpdatesObject(t *testing.T) {
	client := newClient(newPod())
	doc := []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n  namespace: flux-system\n  labels:\n    app: edited\n")

	detail, err := Apply(context.Background(), client, podRef(), doc)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if detail.Labels["app"] != "edited" {
		t.Fatalf("labels = %v, want app=edited", detail.Labels)
	}

	stored, getErr := client.Resource(podGVR).Namespace("flux-system").Get(context.Background(), "web", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("read back: %v", getErr)
	}
	if stored.GetLabels()["app"] != "edited" {
		t.Fatalf("stored labels = %v", stored.GetLabels())
	}
}

func TestApplyRejectsBadYAML(t *testing.T) {
	_, err := Apply(context.Background(), newClient(newPod()), podRef(), []byte("\tnot: [yaml"))
	if err == nil {
		t.Fatalf("expected a parse error")
	}
	if !strings.Contains(err.Error(), "parse yaml") {
		t.Fatalf("error = %v, want a parse yaml error", err)
	}
}

func TestApplyRejectsNameMismatch(t *testing.T) {
	doc := []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: other\n  namespace: flux-system\n")
	_, err := Apply(context.Background(), newClient(newPod()), podRef(), doc)
	if err == nil {
		t.Fatalf("expected a name mismatch error")
	}
	if !strings.Contains(err.Error(), "document name") {
		t.Fatalf("error = %v, want a name mismatch", err)
	}
}

func TestApplyRejectsNamespaceMismatch(t *testing.T) {
	doc := []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n  namespace: default\n")
	_, err := Apply(context.Background(), newClient(newPod()), podRef(), doc)
	if err == nil {
		t.Fatalf("expected a namespace mismatch error")
	}
	if !strings.Contains(err.Error(), "document namespace") {
		t.Fatalf("error = %v, want a namespace mismatch", err)
	}
}

func TestApplyPropagatesAPIError(t *testing.T) {
	doc := []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n  namespace: flux-system\n")
	_, err := Apply(context.Background(), newClient(), podRef(), doc)
	if err == nil {
		t.Fatalf("expected an update error for a missing object")
	}
}

func TestDeleteRemovesObject(t *testing.T) {
	client := newClient(newPod())
	err := Delete(context.Background(), client, podRef())
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, getErr := client.Resource(podGVR).Namespace("flux-system").Get(context.Background(), "web", metav1.GetOptions{})
	if getErr == nil {
		t.Fatalf("object still present after delete")
	}
}

func TestDeleteMissingObject(t *testing.T) {
	err := Delete(context.Background(), newClient(), podRef())
	if err == nil {
		t.Fatalf("expected an error deleting a missing object")
	}
}

func TestConditionFallsBackToLastUpdateTime(t *testing.T) {
	pod := newPod()
	conditions := []any{
		map[string]any{
			"type":           "Ready",
			"status":         "True",
			"lastUpdateTime": "2026-07-27T10:00:00Z",
		},
		"not-a-map",
	}
	if err := unstructured.SetNestedSlice(pod.Object, conditions, "status", "conditions"); err != nil {
		t.Fatalf("set conditions: %v", err)
	}

	detail, err := Get(context.Background(), newClient(pod), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(detail.Conditions) != 1 {
		t.Fatalf("conditions = %v, want 1", detail.Conditions)
	}
	if detail.Conditions[0].Updated != "2026-07-27T10:00:00Z" {
		t.Fatalf("updated = %q", detail.Conditions[0].Updated)
	}
}

func TestContainerNamesSkipMalformedEntries(t *testing.T) {
	pod := newPod()
	containers := []any{
		"not-a-map",
		map[string]any{"image": "no-name"},
		map[string]any{"name": "app"},
	}
	if err := unstructured.SetNestedSlice(pod.Object, containers, "spec", "containers"); err != nil {
		t.Fatalf("set containers: %v", err)
	}
	unstructured.RemoveNestedField(pod.Object, "spec", "initContainers")

	detail, err := Get(context.Background(), newClient(pod), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.Join(detail.Containers, ",") != "app" {
		t.Fatalf("containers = %v, want [app]", detail.Containers)
	}
}

func TestContainerNamesEmptyWhenNoneNamed(t *testing.T) {
	pod := newPod()
	unstructured.RemoveNestedField(pod.Object, "spec", "initContainers")
	unstructured.RemoveNestedField(pod.Object, "spec", "containers")

	detail, err := Get(context.Background(), newClient(pod), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Containers != nil {
		t.Fatalf("containers = %v, want nil", detail.Containers)
	}
}

var _ dynamic.Interface = (*fake.FakeDynamicClient)(nil)

func TestSuspendedReportsTheSpecField(t *testing.T) {
	pod := newPod()
	if err := unstructured.SetNestedField(pod.Object, true, "spec", "suspend"); err != nil {
		t.Fatalf("set suspend: %v", err)
	}

	detail, err := Get(context.Background(), newClient(pod), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Suspended == nil {
		t.Fatalf("suspended = nil, want true")
	}
	if !*detail.Suspended {
		t.Fatalf("suspended = false, want true")
	}
}

func TestSuspendedDistinguishesFalseFromAbsent(t *testing.T) {
	pod := newPod()
	if err := unstructured.SetNestedField(pod.Object, false, "spec", "suspend"); err != nil {
		t.Fatalf("set suspend: %v", err)
	}

	detail, err := Get(context.Background(), newClient(pod), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Suspended == nil {
		t.Fatalf("suspended = nil, want an explicit false")
	}
	if *detail.Suspended {
		t.Fatalf("suspended = true, want false")
	}

	absent, absentErr := Get(context.Background(), newClient(newPod()), podRef())
	if absentErr != nil {
		t.Fatalf("get: %v", absentErr)
	}
	if absent.Suspended != nil {
		t.Fatalf("suspended = %v, want nil when the field is absent", *absent.Suspended)
	}
}

func TestSuspendedIgnoresNonBool(t *testing.T) {
	pod := newPod()
	if err := unstructured.SetNestedField(pod.Object, "yes", "spec", "suspend"); err != nil {
		t.Fatalf("set suspend: %v", err)
	}

	detail, err := Get(context.Background(), newClient(pod), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Suspended != nil {
		t.Fatalf("suspended = %v, want nil for a non-bool field", *detail.Suspended)
	}
}

func TestPortsForAPod(t *testing.T) {
	pod := newPod()
	containers := []any{
		map[string]any{
			"name": "app",
			"ports": []any{
				map[string]any{"name": "http", "containerPort": int64(8080), "protocol": "TCP"},
				map[string]any{"name": "metrics", "containerPort": int64(9090)},
				map[string]any{"name": "dns", "containerPort": int64(53), "protocol": "UDP"},
				map[string]any{"name": "broken"},
				"not-a-map",
			},
		},
		map[string]any{
			"name":  "sidecar",
			"ports": []any{map[string]any{"containerPort": float64(15000)}},
		},
		"not-a-map",
	}
	if err := unstructured.SetNestedSlice(pod.Object, containers, "spec", "containers"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	detail, err := Get(context.Background(), newClient(pod), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if len(detail.Ports) != 3 {
		t.Fatalf("ports = %+v, want http, metrics and the sidecar port", detail.Ports)
	}
	if detail.Ports[0].Name != "http" || detail.Ports[0].Port != 8080 || detail.Ports[0].Protocol != "TCP" {
		t.Fatalf("first port = %+v", detail.Ports[0])
	}
	if detail.Ports[2].Port != 15000 {
		t.Fatalf("sidecar port = %+v", detail.Ports[2])
	}
	for _, port := range detail.Ports {
		if port.Protocol == "UDP" {
			t.Fatalf("a udp port was offered for forwarding: %+v", port)
		}
	}
}

func TestPortsForAPodWithoutAny(t *testing.T) {
	detail, err := Get(context.Background(), newClient(newPod()), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Ports != nil {
		t.Fatalf("ports = %+v, want nil", detail.Ports)
	}
}

func TestPortsForAService(t *testing.T) {
	svc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "prometheus", "namespace": "flux-system"},
		"spec": map[string]any{
			"ports": []any{
				map[string]any{"name": "http", "port": int64(9090), "protocol": "TCP"},
				map[string]any{"name": "gossip", "port": int64(7946), "protocol": "UDP"},
			},
		},
	}}
	ref := api.ObjectRef{Version: "v1", Resource: "services", Namespace: "flux-system", Name: "prometheus"}

	detail, err := Get(context.Background(), newClient(svc), ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if len(detail.Ports) != 1 {
		t.Fatalf("ports = %+v, want only the tcp port", detail.Ports)
	}
	if detail.Ports[0].Port != 9090 {
		t.Fatalf("port = %+v", detail.Ports[0])
	}
}

func TestPortsForAServiceWithoutAny(t *testing.T) {
	svc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "headless", "namespace": "flux-system"},
		"spec":       map[string]any{},
	}}
	ref := api.ObjectRef{Version: "v1", Resource: "services", Namespace: "flux-system", Name: "headless"}

	detail, err := Get(context.Background(), newClient(svc), ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Ports != nil {
		t.Fatalf("ports = %+v, want nil", detail.Ports)
	}
}

func TestPortsIgnoredForOtherKinds(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "p-mk1"},
		"spec":       map[string]any{"ports": []any{map[string]any{"port": int64(10250)}}},
	}}
	ref := api.ObjectRef{Version: "v1", Resource: "nodes", Name: "p-mk1"}

	detail, err := Get(context.Background(), newClient(node), ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Ports != nil {
		t.Fatalf("ports = %+v, want nil for a node", detail.Ports)
	}
}

func TestPortsRejectOutOfRangeNumbers(t *testing.T) {
	pod := newPod()
	containers := []any{
		map[string]any{
			"name": "app",
			"ports": []any{
				map[string]any{"containerPort": int64(70000)},
				map[string]any{"containerPort": int64(-1)},
				map[string]any{"containerPort": int64(8080)},
			},
		},
	}
	if err := unstructured.SetNestedSlice(pod.Object, containers, "spec", "containers"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	detail, err := Get(context.Background(), newClient(pod), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if len(detail.Ports) != 1 {
		t.Fatalf("ports = %+v, want only the in-range port", detail.Ports)
	}
	if detail.Ports[0].Port != 8080 {
		t.Fatalf("port = %+v", detail.Ports[0])
	}
}

func TestReplicasReportsTheSpecField(t *testing.T) {
	deployment := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "web", "namespace": "flux-system"},
		"spec":       map[string]any{"replicas": int64(3)},
	}}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		},
		deployment,
	)
	ref := api.ObjectRef{Group: "apps", Version: "v1", Resource: "deployments", Namespace: "flux-system", Name: "web"}

	detail, err := Get(context.Background(), client, ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Replicas == nil {
		t.Fatal("replicas = nil, want 3")
	}
	if *detail.Replicas != 3 {
		t.Fatalf("replicas = %d, want 3", *detail.Replicas)
	}
}

func TestReplicasIsAbsentForAPod(t *testing.T) {
	detail, err := Get(context.Background(), newClient(newPod()), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Replicas != nil {
		t.Fatalf("replicas = %d, want nil", *detail.Replicas)
	}
}

func TestSchedulableIsTrueForAPlainNode(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "worker-1"},
	}}

	detail, err := Get(context.Background(), newClient(node), api.ObjectRef{Version: "v1", Resource: "nodes", Name: "worker-1"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Schedulable == nil {
		t.Fatal("schedulable = nil, want true")
	}
	if !*detail.Schedulable {
		t.Fatal("schedulable = false, want true")
	}
}

func TestSchedulableIsFalseForACordonedNode(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "worker-1"},
		"spec":       map[string]any{"unschedulable": true},
	}}

	detail, err := Get(context.Background(), newClient(node), api.ObjectRef{Version: "v1", Resource: "nodes", Name: "worker-1"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Schedulable == nil || *detail.Schedulable {
		t.Fatalf("schedulable = %v, want false", detail.Schedulable)
	}
}

func TestSchedulableIsAbsentForANonNode(t *testing.T) {
	detail, err := Get(context.Background(), newClient(newPod()), podRef())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Schedulable != nil {
		t.Fatalf("schedulable = %v, want nil for a pod", *detail.Schedulable)
	}
}

func TestSchedulableIgnoresANonBoolField(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "worker-1"},
		"spec":       map[string]any{"unschedulable": "yes"},
	}}

	detail, err := Get(context.Background(), newClient(node), api.ObjectRef{Version: "v1", Resource: "nodes", Name: "worker-1"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Schedulable != nil {
		t.Fatalf("schedulable = %v, want nil", *detail.Schedulable)
	}
}
