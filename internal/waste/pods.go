package waste

import (
	"context"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	bytesPerMi = 1024 * 1024
	ownerDepth = 4
	podKind    = "Pod"
)

var ownerKinds = []api.ObjectRef{
	{Group: "apps", Version: "v1", Resource: "replicasets"},
	{Group: "batch", Version: "v1", Resource: "jobs"},
}

func ownerIndex(
	ctx context.Context,
	lister Lister,
	namespace string,
) (map[string]*unstructured.Unstructured, []string) {
	held := map[string]*unstructured.Unstructured{}
	notes := []string{}
	for _, kind := range ownerKinds {
		ref := kind
		ref.Namespace = namespace
		found, err := lister.ListKind(ctx, ref)
		if err != nil {
			notes = append(notes, ref.Resource+
				" could not be read, so some workloads are named by the pod's own owner: "+err.Error())
			continue
		}
		for _, obj := range found {
			held[ownerKey(obj.GroupVersionKind().Group, obj.GetKind(), obj.GetNamespace(), obj.GetName())] = obj
		}
	}
	return held, notes
}

func ownerKey(group, kind, namespace, name string) string {
	return group + "/" + kind + "/" + namespace + "/" + name
}

func topOwner(obj *unstructured.Unstructured, held map[string]*unstructured.Unstructured) (string, string) {
	kind := obj.GetKind()
	if kind == "" {
		kind = podKind
	}
	name := obj.GetName()
	current := obj
	for range ownerDepth {
		ref, ok := controllerOf(current)
		if !ok {
			return kind, name
		}
		kind = ref.Kind
		name = ref.Name
		next, found := held[ownerKey(groupOf(ref.APIVersion), ref.Kind, current.GetNamespace(), ref.Name)]
		if !found {
			return kind, name
		}
		current = next
	}
	return kind, name
}

func controllerOf(obj *unstructured.Unstructured) (metav1.OwnerReference, bool) {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		return ref, true
	}
	return metav1.OwnerReference{}, false
}

func groupOf(apiVersion string) string {
	group, _, found := strings.Cut(apiVersion, "/")
	if !found {
		return ""
	}
	return group
}

func requestsOf(obj *unstructured.Unstructured) (int64, int64, bool) {
	var cpuMilli, memMi int64
	asks := false
	for _, raw := range unstr.Slice(obj, "spec", "containers") {
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		cpu, hasCPU := requestedIn(container, "cpu")
		if hasCPU {
			cpuMilli += cpu.MilliValue()
			asks = true
		}
		mem, hasMemory := requestedIn(container, "memory")
		if hasMemory {
			memMi += mem.Value() / bytesPerMi
			asks = true
		}
	}
	return cpuMilli, memMi, asks
}

func requestedIn(container map[string]any, name string) (resource.Quantity, bool) {
	held, ok := container["resources"].(map[string]any)
	if !ok {
		return resource.Quantity{}, false
	}
	asked, ok := held["requests"].(map[string]any)
	if !ok {
		return resource.Quantity{}, false
	}
	return quantityFrom(asked[name])
}

func quantityFrom(raw any) (resource.Quantity, bool) {
	switch value := raw.(type) {
	case string:
		return parsed(value)
	case int64:
		return parsed(strconv.FormatInt(value, 10))
	case float64:
		return parsed(strconv.FormatFloat(value, 'f', -1, 64))
	default:
		return resource.Quantity{}, false
	}
}

func parsed(raw string) (resource.Quantity, bool) {
	value, err := resource.ParseQuantity(raw)
	if err != nil {
		return resource.Quantity{}, false
	}
	return value, true
}
