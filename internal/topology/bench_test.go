package topology

import (
	"context"
	"strconv"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func benchWorkload(namespace, name string) []*unstructured.Unstructured {
	labels := map[string]any{"app": name}
	deployment := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   meta(name, namespace, name),
		"spec": map[string]any{
			"replicas": int64(3),
			"template": map[string]any{"spec": map[string]any{
				"volumes": []any{map[string]any{"secret": map[string]any{"secretName": name + "-tls"}}},
				"containers": []any{map[string]any{
					"envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": name + "-config"}}},
				}},
			}},
		},
		"status": map[string]any{"readyReplicas": int64(3)},
	}}
	replicas := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata":   ownedBy(name+"-rs", namespace, name+"-rs", "Deployment", name, name, "apps/v1"),
		"spec":       map[string]any{"replicas": int64(3)},
		"status":     map[string]any{"readyReplicas": int64(3)},
	}}
	service := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   meta(name, namespace, name+"-svc"),
		"spec":       map[string]any{"selector": labels},
	}}
	out := make([]*unstructured.Unstructured, 0, 6)
	out = append(out, deployment, replicas, service)
	for index := range 3 {
		holder := ownedBy(name+"-"+strconv.Itoa(index), namespace, name+"-pod-"+strconv.Itoa(index),
			"ReplicaSet", name+"-rs", name+"-rs", "apps/v1")
		holder["labels"] = labels
		out = append(out, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   holder,
			"status": map[string]any{
				"phase":      "Running",
				"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
			},
		}})
	}
	return out
}

func benchCluster(namespaces, perNamespace int) []runtime.Object {
	objects := []runtime.Object{}
	for space := range namespaces {
		namespace := "ns-" + strconv.Itoa(space)
		for index := range perNamespace {
			for _, obj := range benchWorkload(namespace, namespace+"-app-"+strconv.Itoa(index)) {
				objects = append(objects, obj)
			}
		}
	}
	return objects
}

// The dynamic fake deep-copies its whole tracker on every List, which would
// dominate the measurement. This one hands back what an informer cache holds.
type staticLister struct {
	byResource map[string][]*unstructured.Unstructured
}

func staticFrom(objects []runtime.Object) *staticLister {
	byResource := map[string][]*unstructured.Unstructured{}
	plural := map[string]string{
		"Deployment": "deployments",
		"ReplicaSet": "replicasets",
		"Pod":        "pods",
		"Service":    "services",
	}
	for _, one := range objects {
		item, ok := one.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		resource := plural[item.GetKind()]
		byResource[resource] = append(byResource[resource], item)
	}
	return &staticLister{byResource: byResource}
}

func (s *staticLister) List(_ context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	return s.byResource[desc.Resource], nil
}

func (s *staticLister) Warm(context.Context, []api.ResourceDescriptor) {}

func benchmarkBuild(b *testing.B, namespaces, perNamespace int) {
	b.Helper()
	lister := staticFrom(benchCluster(namespaces, perNamespace))
	descriptors := descs()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		Build(context.Background(), lister, descriptors, Request{})
	}
}

func BenchmarkBuildAcrossTenNamespaces(b *testing.B) {
	benchmarkBuild(b, 10, 10)
}

func BenchmarkBuildOneCrowdedNamespace(b *testing.B) {
	benchmarkBuild(b, 1, 400)
}

func BenchmarkBuildALargeCluster(b *testing.B) {
	benchmarkBuild(b, 40, 50)
}
