package issues

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var testNow = time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

const (
	phasePending = "Pending"
	phaseRunning = "Running"
)

type stubLister struct {
	mu     sync.Mutex
	items  map[string][]*unstructured.Unstructured
	errs   map[string]error
	cached []api.ResourceDescriptor
	leased []api.ResourceDescriptor
}

func (s *stubLister) Lease(
	_ context.Context,
	desc api.ResourceDescriptor,
) ([]*unstructured.Unstructured, error) {
	s.mu.Lock()
	s.leased = append(s.leased, desc)
	s.mu.Unlock()
	err, refused := s.errs[desc.Resource]
	if refused {
		return nil, err
	}
	return s.items[desc.Resource], nil
}

func (s *stubLister) Cached() []api.ResourceDescriptor {
	return s.cached
}

type stubEvents struct {
	mu    sync.Mutex
	byUID map[string][]api.Event
	err   error
	asked []string
}

func (s *stubEvents) Events(_ context.Context, _, uid string) ([]api.Event, error) {
	s.mu.Lock()
	s.asked = append(s.asked, uid)
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return s.byUID[uid], nil
}

func (s *stubEvents) askedAbout() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := slices.Clone(s.asked)
	slices.Sort(out)
	return out
}

func descriptor(group, version, resource, kind string) api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      group,
		Version:    version,
		Resource:   resource,
		Kind:       kind,
		Namespaced: true,
	}
}

func podDescriptor() api.ResourceDescriptor {
	return descriptor("", "v1", "pods", kindPod)
}

func replicaSetDescriptor() api.ResourceDescriptor {
	return descriptor(appsGroup, "v1", "replicasets", kindReplicaSet)
}

func deploymentDescriptor() api.ResourceDescriptor {
	return descriptor(appsGroup, "v1", "deployments", kindDeployment)
}

func catalog(descs ...api.ResourceDescriptor) map[string]api.ResourceDescriptor {
	out := map[string]api.ResourceDescriptor{}
	for _, desc := range descs {
		out[keyOf(desc)] = desc
	}
	return out
}

type podOption func(pod *unstructured.Unstructured)

func newPod(name string, options ...podOption) *unstructured.Unstructured {
	pod := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       kindPod,
		"metadata": map[string]any{
			"name":              name,
			"namespace":         "default",
			"uid":               "uid-" + name,
			"creationTimestamp": testNow.Add(-time.Hour).Format(time.RFC3339),
		},
		"spec":   map[string]any{"nodeName": "node-a"},
		"status": map[string]any{"phase": phaseRunning},
	}}
	for _, apply := range options {
		apply(pod)
	}
	return pod
}

func withPhase(phase string) podOption {
	return func(pod *unstructured.Unstructured) {
		setNested(pod, phase, "status", "phase")
	}
}

func withContainer(name string, state map[string]any) podOption {
	return func(pod *unstructured.Unstructured) {
		entry := map[string]any{"name": name, "state": state}
		appendNested(pod, entry, "status", "containerStatuses")
	}
}

func withContainerEntry(entry map[string]any) podOption {
	return func(pod *unstructured.Unstructured) {
		appendNested(pod, entry, "status", "containerStatuses")
	}
}

func withOwner(kind, name, uid string) podOption {
	return func(pod *unstructured.Unstructured) {
		controller := true
		pod.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: "apps/v1",
			Kind:       kind,
			Name:       name,
			UID:        types.UID(uid),
			Controller: &controller,
		}})
	}
}

func withStartTime(at time.Time) podOption {
	return func(pod *unstructured.Unstructured) {
		setNested(pod, at.Format(time.RFC3339), "status", "startTime")
	}
}

func withNode(name string) podOption {
	return func(pod *unstructured.Unstructured) {
		setNested(pod, name, "spec", "nodeName")
	}
}

func withPodCondition(entry map[string]any) podOption {
	return func(pod *unstructured.Unstructured) {
		appendNested(pod, entry, "status", "conditions")
	}
}

func withDeleted() podOption {
	return func(pod *unstructured.Unstructured) {
		stamp := metav1.NewTime(testNow)
		pod.SetDeletionTimestamp(&stamp)
	}
}

func setNested(obj *unstructured.Unstructured, value any, fields ...string) {
	err := unstructured.SetNestedField(obj.Object, value, fields...)
	if err != nil {
		panic(err)
	}
}

func appendNested(obj *unstructured.Unstructured, entry map[string]any, fields ...string) {
	existing, _, _ := unstructured.NestedSlice(obj.Object, fields...)
	err := unstructured.SetNestedSlice(obj.Object, append(existing, entry), fields...)
	if err != nil {
		panic(err)
	}
}

func newWorkload(kind, name, uid string, status, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       kind,
		"metadata": map[string]any{
			"name":              name,
			"namespace":         "default",
			"uid":               uid,
			"generation":        int64(3),
			"creationTimestamp": testNow.Add(-time.Hour).Format(time.RFC3339),
		},
		"spec":   spec,
		"status": status,
	}}
}

func condition(kind, status string, extras map[string]any) map[string]any {
	entry := map[string]any{
		"type":               kind,
		"status":             status,
		"lastTransitionTime": testNow.Add(-30 * time.Minute).Format(time.RFC3339),
	}
	maps.Copy(entry, extras)
	return entry
}

func build(t *testing.T, lister *stubLister, descs map[string]api.ResourceDescriptor) api.IssueQueue {
	t.Helper()
	return Build(t.Context(), lister, &stubEvents{}, descs, func() time.Time { return testNow })
}

func rowNamed(queue api.IssueQueue, name string) (api.Issue, bool) {
	for _, row := range queue.Rows {
		if row.Object.Name == name {
			return row, true
		}
	}
	return api.Issue{}, false
}

func itemsOf(resource string, objs ...*unstructured.Unstructured) map[string][]*unstructured.Unstructured {
	return map[string][]*unstructured.Unstructured{resource: objs}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func metaNow() metav1.Time {
	return metav1.NewTime(testNow)
}

func ownerReference(kind, name, uid string, controller *bool) []metav1.OwnerReference {
	return []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       kind,
		Name:       name,
		UID:        types.UID(uid),
		Controller: controller,
	}}
}

func snapshotOf(items ...object) *snapshot {
	snap := newSnapshot()
	for _, item := range items {
		snap.byUID[item.uid()] = item
		key := item.desc.Group + "/" + item.desc.Kind
		snap.byKind[key] = append(snap.byKind[key], item)
		owner := controllerUID(item.obj)
		if owner == "" {
			continue
		}
		snap.byOwner[owner] = append(snap.byOwner[owner], item)
	}
	return snap
}
