package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

type watchedType struct {
	kind   string
	lister cache.GenericLister
}

func (m *Manager) failingFromCaches() map[string]int {
	watched := m.watchedTypes()
	out := map[string]int{}
	for key, entry := range watched {
		count := failingInCache(entry)
		if count == 0 {
			continue
		}
		out[key] = count
	}
	return out
}

func (m *Manager) watchedTypes() map[string]watchedType {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]watchedType{}
	for key, st := range m.streams {
		if key.ns != "" {
			continue
		}
		if st.kind == "Pod" {
			continue
		}
		if !st.informer.HasSynced() {
			continue
		}
		out[gvrKey(key.gvr)] = watchedType{kind: st.kind, lister: st.lister}
	}
	return out
}

func gvrKey(gvr schema.GroupVersionResource) string {
	return gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
}

func failingInCache(entry watchedType) int {
	listed, err := entry.lister.List(labels.Everything())
	if err != nil {
		return 0
	}
	count := 0
	for _, obj := range listed {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		if unhealthy(u, entry.kind) {
			count++
		}
	}
	return count
}

func unhealthy(obj *unstructured.Unstructured, kind string) bool {
	switch kind {
	case "Pod":
		return false
	case "Deployment", "StatefulSet", "ReplicaSet", "ReplicationController":
		return unstr.Int(obj, "status", "readyReplicas") < unstr.Int(obj, "spec", "replicas")
	case "DaemonSet":
		return unstr.Int(obj, "status", "numberReady") < unstr.Int(obj, "status", "desiredNumberScheduled")
	case "Job":
		return conditionTrue(obj, "Failed")
	default:
		status, _ := unstr.Ready(obj)
		if status == "" {
			return false
		}
		return status != "True"
	}
}

func conditionTrue(u *unstructured.Unstructured, name string) bool {
	for _, raw := range unstr.Slice(u, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if unstr.At(entry, "type") != name {
			continue
		}
		return unstr.At(entry, "status") == "True"
	}
	return false
}
