package checks

import (
	"context"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

type fakeLister struct {
	mu      sync.Mutex
	objects map[string][]*unstructured.Unstructured
	errs    map[string]error
	warmed  int
}

func newLister(objects ...*unstructured.Unstructured) *fakeLister {
	held := map[string][]*unstructured.Unstructured{}
	for _, obj := range objects {
		resource := resourceFor(obj.GetKind())
		held[resource] = append(held[resource], obj)
	}
	return &fakeLister{objects: held, errs: map[string]error{}}
}

func (f *fakeLister) List(_ context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	err, failing := f.errs[desc.Resource]
	if failing {
		return nil, err
	}
	return f.objects[desc.Resource], nil
}

func (f *fakeLister) Warm(context.Context, []api.ResourceDescriptor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.warmed++
}

func (f *fakeLister) warmCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.warmed
}

var kindResources = map[string]string{
	"Pod":                   "pods",
	"Deployment":            "deployments",
	"StatefulSet":           "statefulsets",
	"DaemonSet":             "daemonsets",
	"ReplicaSet":            "replicasets",
	"ReplicationController": "replicationcontrollers",
	"Job":                   "jobs",
	"CronJob":               "cronjobs",
}

var kindGroups = map[string]string{
	"Deployment":  "apps",
	"StatefulSet": "apps",
	"DaemonSet":   "apps",
	"ReplicaSet":  "apps",
	"Job":         "batch",
	"CronJob":     "batch",
}

func resourceFor(kind string) string {
	return kindResources[kind]
}

func descriptors() map[string]api.ResourceDescriptor {
	out := map[string]api.ResourceDescriptor{}
	for kind, resource := range kindResources {
		group := kindGroups[kind]
		version := "v1"
		out[discovery.Key(group, version, resource)] = api.ResourceDescriptor{
			Group:      group,
			Version:    version,
			Resource:   resource,
			Kind:       kind,
			Namespaced: true,
			Category:   "Workloads",
		}
	}
	return out
}

func report(t *testing.T, objects ...*unstructured.Unstructured) api.CheckReport {
	t.Helper()
	return Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{})
}

func reportWithUsage(
	t *testing.T,
	usage map[string]api.ResourceUsage,
	objects ...*unstructured.Unstructured,
) api.CheckReport {
	t.Helper()
	return Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{Pods: usage})
}

func groupNamed(t *testing.T, found api.CheckReport, id string) api.CheckGroup {
	t.Helper()
	for _, group := range found.Groups {
		if group.ID == id {
			return group
		}
	}
	t.Fatalf("no check named %q in the report", id)
	return api.CheckGroup{}
}

func onlyFinding(t *testing.T, found api.CheckReport, id string) api.CheckFinding {
	t.Helper()
	group := groupNamed(t, found, id)
	if len(group.Findings) != 1 {
		t.Fatalf("%s produced %d findings, want 1: %+v", id, len(group.Findings), group.Findings)
	}
	return group.Findings[0]
}

func objectFor(t *testing.T, found api.CheckReport, finding api.CheckFinding) api.CheckObject {
	t.Helper()
	if finding.Ref < 0 || finding.Ref >= len(found.Objects) {
		t.Fatalf("ref %d falls outside the %d objects the report carries", finding.Ref, len(found.Objects))
	}
	return found.Objects[finding.Ref]
}

func onlyObject(t *testing.T, found api.CheckReport, id string) api.CheckObject {
	t.Helper()
	return objectFor(t, found, onlyFinding(t, found, id))
}

func findingCount(t *testing.T, found api.CheckReport, id string) int {
	t.Helper()
	return len(groupNamed(t, found, id).Findings)
}
