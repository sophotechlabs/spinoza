package resources

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

const informerBenchmarkObjectCount = 5000

func benchmarkInformerPod(tb testing.TB, index int) *unstructured.Unstructured {
	tb.Helper()
	name := fmt.Sprintf("checkout-%05d", index)
	namespace := "team-" + strconv.Itoa(index%50)
	lastApplied := fmt.Sprintf(
		`{"apiVersion":"v1","kind":"Pod","metadata":{"name":%q,"namespace":%q},"spec":{"containers":[{"name":"app","image":"registry.example.com/shop/checkout:v42","env":%q}]}}`,
		name,
		namespace,
		strings.Repeat("configured-value-", 96),
	)
	fields := map[string]any{
		"f:metadata": map[string]any{
			"f:annotations": map[string]any{
				".": map[string]any{},
				"f:kubectl.kubernetes.io/last-applied-configuration": map[string]any{},
			},
			"f:labels": map[string]any{
				".":                        map[string]any{},
				"f:app.kubernetes.io/name": map[string]any{},
				"f:team":                   map[string]any{},
			},
		},
		"f:spec": map[string]any{
			"f:containers": map[string]any{
				"k:{\"name\":\"app\"}": map[string]any{
					".":           map[string]any{},
					"f:env":       map[string]any{},
					"f:image":     map[string]any{},
					"f:name":      map[string]any{},
					"f:resources": map[string]any{},
				},
			},
			"f:nodeName":           map[string]any{},
			"f:serviceAccountName": map[string]any{},
			"f:volumes":            map[string]any{},
		},
	}
	value := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"uid":       "4b7b7d42-" + name,
			"labels": map[string]any{
				"app.kubernetes.io/name":       "checkout",
				"app.kubernetes.io/instance":   "shop",
				"app.kubernetes.io/managed-by": "Helm",
				"pod-template-hash":            strconv.Itoa(1000000000 + index),
				"team":                         namespace,
			},
			"annotations": map[string]any{
				"checksum/config": fmt.Sprintf("%064d", index),
				"kubectl.kubernetes.io/last-applied-configuration": lastApplied,
				"prometheus.io/path":                               "/metrics",
				"prometheus.io/port":                               "9090",
				"prometheus.io/scrape":                             "true",
			},
			"managedFields": []any{
				map[string]any{
					"apiVersion": "v1",
					"fieldsType": "FieldsV1",
					"fieldsV1":   fields,
					"manager":    "kube-controller-manager",
					"operation":  "Update",
				},
				map[string]any{
					"apiVersion":  "v1",
					"fieldsType":  "FieldsV1",
					"fieldsV1":    fields,
					"manager":     "kubelet",
					"operation":   "Update",
					"subresource": "status",
				},
			},
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"env": []any{
						map[string]any{"name": "LOG_LEVEL", "value": "info"},
						map[string]any{"name": "REGION", "value": "eu-central-1"},
						map[string]any{"name": "WORKERS", "value": "8"},
					},
					"image": "registry.example.com/shop/checkout:v42",
					"name":  "app",
					"ports": []any{
						map[string]any{"containerPort": float64(8080), "name": "http", "protocol": "TCP"},
						map[string]any{"containerPort": float64(9090), "name": "metrics", "protocol": "TCP"},
					},
					"resources": map[string]any{
						"limits":   map[string]any{"cpu": "1", "memory": "1Gi"},
						"requests": map[string]any{"cpu": "250m", "memory": "256Mi"},
					},
					"volumeMounts": []any{
						map[string]any{"mountPath": "/etc/shop", "name": "config", "readOnly": true},
						map[string]any{"mountPath": "/var/run/secrets/shop", "name": "credentials", "readOnly": true},
					},
				},
			},
			"nodeName":           "worker-" + strconv.Itoa(index%200),
			"serviceAccountName": "checkout",
			"volumes": []any{
				map[string]any{"configMap": map[string]any{"name": "checkout"}, "name": "config"},
				map[string]any{"name": "credentials", "secret": map[string]any{"secretName": "checkout"}},
			},
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"status": "True", "type": "PodReadyToStartContainers"},
				map[string]any{"status": "True", "type": "Initialized"},
				map[string]any{"status": "True", "type": "Ready"},
				map[string]any{"status": "True", "type": "ContainersReady"},
				map[string]any{"status": "True", "type": "PodScheduled"},
			},
			"containerStatuses": []any{
				map[string]any{
					"containerID":  "containerd://" + fmt.Sprintf("%064d", index),
					"image":        "registry.example.com/shop/checkout:v42",
					"imageID":      "registry.example.com/shop/checkout@sha256:" + fmt.Sprintf("%064d", index),
					"name":         "app",
					"ready":        true,
					"restartCount": float64(0),
					"started":      true,
					"state":        map[string]any{"running": map[string]any{"startedAt": "2026-08-29T12:00:00Z"}},
				},
			},
			"hostIP": "10.0." + strconv.Itoa(index%200) + ".10",
			"phase":  "Running",
			"podIP":  "10.244." + strconv.Itoa(index%250) + ".20",
		},
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		tb.Fatalf("marshal benchmark pod: %v", err)
	}
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(encoded); err != nil {
		tb.Fatalf("decode benchmark pod: %v", err)
	}
	return obj
}

func retainedInformerBytes(b *testing.B, transformed bool) uint64 {
	b.Helper()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	for index := range informerBenchmarkObjectCount {
		obj := benchmarkInformerPod(b, index)
		if transformed {
			value, err := stripManagedFields(obj)
			if err != nil {
				b.Fatalf("transform benchmark pod: %v", err)
			}
			var ok bool
			obj, ok = value.(*unstructured.Unstructured)
			if !ok {
				b.Fatalf("transformed object type = %T", value)
			}
		}
		if err := indexer.Add(obj); err != nil {
			b.Fatalf("add benchmark pod: %v", err)
		}
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(indexer)
	if after.HeapAlloc <= before.HeapAlloc {
		b.Fatalf("heap did not grow: before %d, after %d", before.HeapAlloc, after.HeapAlloc)
	}
	return after.HeapAlloc - before.HeapAlloc
}

func benchmarkInformerCacheMemory(b *testing.B, transformed bool) {
	b.Helper()
	var retained uint64
	for b.Loop() {
		retained += retainedInformerBytes(b, transformed)
	}
	average := float64(retained) / float64(b.N)
	b.ReportMetric(informerBenchmarkObjectCount, "objects")
	b.ReportMetric(average, "B/cache")
	b.ReportMetric(average/informerBenchmarkObjectCount, "B/object")
}

func BenchmarkInformerCacheMemoryWithoutTransform(b *testing.B) {
	benchmarkInformerCacheMemory(b, false)
}

func BenchmarkInformerCacheMemoryWithTransform(b *testing.B) {
	benchmarkInformerCacheMemory(b, true)
}
