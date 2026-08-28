package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/issues"
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
	out := map[string]watchedType{}
	for key, entry := range m.syncedTypes() {
		if entry.kind == "Pod" {
			continue
		}
		out[key] = entry
	}
	return out
}

func (m *Manager) syncedTypes() map[string]watchedType {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]watchedType{}
	for key, st := range m.streams {
		if !st.informer.HasSynced() {
			continue
		}
		out[gvrKey(key.gvr)] = watchedType{kind: st.kind, lister: st.lister}
	}
	return out
}

func (m *Manager) Cached() []api.ResourceDescriptor {
	synced := m.syncedTypes()
	descs := m.descriptors()
	out := make([]api.ResourceDescriptor, 0, len(synced))
	for key := range synced {
		desc, ok := descs[key]
		if !ok {
			continue
		}
		out = append(out, desc)
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
	case "Deployment", "StatefulSet", "ReplicaSet", "ReplicationController", "DaemonSet", "Job":
		return issues.WorkloadUnhealthy(obj, kind)
	default:
		status, _ := unstr.Ready(obj)
		if status == "" {
			return false
		}
		return status != "True"
	}
}
