package checks

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func everyShape() []*unstructured.Unstructured {
	owner := deployment("api", hostileWorkload("api"))
	running := ownedBy(onNode(pod("api-a", hostileWorkload("api-a")), "worker-1"), "Deployment", "api")
	standalone := pod("standalone", podSpec(container("app", nil)))
	set := workload("StatefulSet", "db", hostileWorkload("db"))
	daemon := workload("DaemonSet", "agent", hostileWorkload("agent"))
	batch := cronJob("nightly", hostileWorkload("nightly"))
	job := workload("Job", "once", hostileWorkload("once"))
	return []*unstructured.Unstructured{
		owner, running, standalone, set, daemon, batch, job,
		bare("Service", "web"),
		bare("ServiceAccount", "default"),
		bare("ConfigMap", "config"),
		bare("Secret", "creds"),
		bare("Namespace", "shop"),
		bare("Node", "worker-1"),
		bare("Ingress", "front"),
		bare("NetworkPolicy", "deny"),
		bare("PodDisruptionBudget", "web-pdb"),
		bare("HorizontalPodAutoscaler", "web-hpa"),
		bare("PersistentVolumeClaim", "data"),
		bare("ResourceQuota", "quota"),
		bare("LimitRange", "limits"),
		bare("Role", "reader"),
		bare("RoleBinding", "reader-binding"),
		bare("ClusterRole", "admin"),
		bare("ClusterRoleBinding", "admin-binding"),
	}
}

func bare(kind, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersionFor(kind),
		"kind":       kind,
		"metadata":   metadata(name),
	}}
}

func copies(objects []*unstructured.Unstructured) []*unstructured.Unstructured {
	out := make([]*unstructured.Unstructured, 0, len(objects))
	for _, obj := range objects {
		out = append(out, obj.DeepCopy())
	}
	return out
}

func writtenThrough(before, after []*unstructured.Unstructured) string {
	for at := range before {
		if reflect.DeepEqual(before[at].Object, after[at].Object) {
			continue
		}
		return before[at].GetKind() + " " + before[at].GetName()
	}
	return ""
}

func stillIdentical(t *testing.T, before, after []*unstructured.Unstructured, who string) {
	t.Helper()
	touched := writtenThrough(before, after)
	if touched == "" {
		return
	}
	t.Fatalf(
		"%s modified the cached %s; specAt hands out the informer cache without copying",
		who, touched,
	)
}

func TestNoCheckInTheRegistryWritesThroughTheCache(t *testing.T) {
	objects := everyShape()
	before := copies(objects)
	lister := newLister(objects...)
	keep := everyKind()

	sc, _, _ := survey(t.Context(), lister, descriptors(), api.Metrics{}, keep)
	stillIdentical(t, before, objects, "survey")

	entries := registryWith(nil)
	if len(entries) == 0 {
		t.Fatal("the registry came back empty; this guard would pass without checking anything")
	}
	for _, entry := range entries {
		t.Run(entry.id, func(t *testing.T) {
			entry.matching(sc, keep)
			stillIdentical(t, before, objects, entry.id)
		})
	}
}

func TestTheGuardNoticesACheckThatWritesThroughTheCache(t *testing.T) {
	objects := everyShape()
	before := copies(objects)
	lister := newLister(objects...)
	keep := everyKind()
	sc, _, _ := survey(t.Context(), lister, descriptors(), api.Metrics{}, keep)

	for _, subject := range sc.subjects {
		spec := specAt(subject.Object, "spec")
		if len(spec) == 0 {
			continue
		}
		spec["spinozaWroteHere"] = true
		break
	}

	if writtenThrough(before, objects) == "" {
		t.Fatal("writing through specAt went unnoticed, so the guard proves nothing")
	}
}
