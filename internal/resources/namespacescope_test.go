package resources

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

type listCounter struct {
	mu          sync.Mutex
	byNamespace map[string]int
}

func (c *listCounter) seen(namespace string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byNamespace[namespace]
}

func namespaceBoundClient(t *testing.T, allowed map[string]bool, objs ...runtime.Object) (*fake.FakeDynamicClient, *listCounter) {
	t.Helper()
	dyn := newClient(t, objs...)
	counts := &listCounter{byNamespace: map[string]int{}}
	dyn.PrependReactor("list", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		counts.mu.Lock()
		counts.byNamespace[action.GetNamespace()]++
		counts.mu.Unlock()
		if allowed[action.GetNamespace()] {
			return false, nil, nil
		}
		return true, nil, apierrors.NewForbidden(depGVR.GroupResource(), "", errors.New("deployments.apps is forbidden: this account reads named namespaces only"))
	})
	return dyn, counts
}

func scopedManager(t *testing.T, dyn *fake.FakeDynamicClient) *Manager {
	t.Helper()
	mgr, cancel := newManager(t, dyn)
	t.Cleanup(cancel)
	mgr.syncTimeout = time.Second
	return mgr
}

func TestANamespaceOnlyCredentialCanBrowseItsNamespace(t *testing.T) {
	dyn, counts := namespaceBoundClient(t, map[string]bool{"review": true}, newDeployment("review", "first"))
	mgr := scopedManager(t, dyn)

	sub, err := mgr.Subscribe(t.Context(), "apps", "v1", "deployments", "review", 0, nil)
	if err != nil {
		t.Fatalf("a valid namespace list succeeded but the subscription failed: %v", err)
	}
	t.Cleanup(sub.Close)

	if len(sub.Rows) != 1 || sub.Rows[0].Name != "first" {
		t.Fatalf("rows = %+v, want the one deployment the account may read", sub.Rows)
	}
	if counts.seen("") != 1 {
		t.Fatalf("cluster-wide lists = %d, want exactly one attempt before scoping down", counts.seen(""))
	}
	if counts.seen("review") != 1 {
		t.Fatalf("namespace lists = %d, want one informer for the namespace", counts.seen("review"))
	}

	_, err = dyn.Resource(depGVR).Namespace("review").Create(context.Background(), newDeployment("review", "second"), metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ev := recvEvent(t, sub.Events)
	if ev.Kind != "added" || ev.Row.Name != "second" {
		t.Fatalf("event = %+v, want the deployment that just appeared", ev)
	}
}

func TestANamespaceOnlyCredentialIsStillRefusedElsewhere(t *testing.T) {
	dyn, _ := namespaceBoundClient(t, map[string]bool{"review": true})
	mgr := scopedManager(t, dyn)

	_, err := mgr.Subscribe(t.Context(), "apps", "v1", "deployments", "other", 0, nil)

	if !apierrors.IsForbidden(err) {
		t.Fatalf("error = %v, want the cluster's own refusal for the other namespace", err)
	}
}

func TestANamespaceOnlyCredentialMustPickANamespaceForTheWholeCluster(t *testing.T) {
	dyn, _ := namespaceBoundClient(t, map[string]bool{"review": true})
	mgr := scopedManager(t, dyn)

	_, err := mgr.Subscribe(t.Context(), "apps", "v1", "deployments", "", 0, nil)

	if !errors.Is(err, ErrNamespaceNeeded) {
		t.Fatalf("error = %v, want ErrNamespaceNeeded", err)
	}
	want := "your account cannot read this kind across every namespace; pick one namespace: Deployment: "
	if !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("error = %q, want it to start with %q", err.Error(), want)
	}
	if !apierrors.IsForbidden(err) || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error = %q, want the cluster's own denial kept inside it", err.Error())
	}
}

func TestAKindInNoNamespaceKeepsTheClustersOwnRefusal(t *testing.T) {
	dyn := newClient(t)
	dyn.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(nodeGVR.GroupResource(), "", errors.New("nodes is forbidden"))
	})
	mgr := scopedManager(t, dyn)

	_, err := mgr.Subscribe(t.Context(), "", "v1", "nodes", "", 0, nil)

	if !apierrors.IsForbidden(err) || errors.Is(err, ErrNamespaceNeeded) {
		t.Fatalf("error = %v, want the plain refusal, since no namespace could help", err)
	}
}

func TestScopedStreamsAreSharedPerNamespaceNotPerSubscriber(t *testing.T) {
	dyn, counts := namespaceBoundClient(t, map[string]bool{"a": true, "b": true},
		newDeployment("a", "one"), newDeployment("b", "two"))
	mgr := scopedManager(t, dyn)

	for _, namespace := range []string{"a", "b", "a"} {
		sub, err := mgr.Subscribe(t.Context(), "apps", "v1", "deployments", namespace, 0, nil)
		if err != nil {
			t.Fatalf("subscribe %s: %v", namespace, err)
		}
		t.Cleanup(sub.Close)
	}

	if counts.seen("a") != 1 || counts.seen("b") != 1 {
		t.Fatalf("lists = a:%d b:%d, want one informer per namespace", counts.seen("a"), counts.seen("b"))
	}
	if streamCount(mgr) != 2 {
		t.Fatalf("streams = %d, want one per namespace the account can read", streamCount(mgr))
	}
}

func TestAScopedStreamIsNotOfferedAsTheWholeKind(t *testing.T) {
	dyn, _ := namespaceBoundClient(t, map[string]bool{"review": true}, newDeployment("review", "first"))
	mgr := scopedManager(t, dyn)
	sub, err := mgr.Subscribe(t.Context(), "apps", "v1", "deployments", "review", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)

	if _, offered := mgr.syncedTypes()[gvrKey(depGVR)]; offered {
		t.Fatal("a namespace-scoped informer was offered as a view of the whole kind")
	}
}

func TestEqualTimestampsSelectTheSameWindowWhateverTheInputOrder(t *testing.T) {
	first := newDeployment("review", "first")
	second := newDeployment("review", "second")
	stamp := metav1.NewTime(time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC))
	first.SetCreationTimestamp(stamp)
	second.SetCreationTimestamp(stamp)

	left := newestFirst("Deployment", []*unstructured.Unstructured{first, second}, 1)
	right := newestFirst("Deployment", []*unstructured.Unstructured{second, first}, 1)

	if left[0].GetUID() != right[0].GetUID() {
		t.Fatalf("identical objects selected different windows: %s versus %s", left[0].GetName(), right[0].GetName())
	}
	if left[0].GetName() != "first" {
		t.Fatalf("window = %s, want the alphabetically first of two equal-time objects", left[0].GetName())
	}
}

func TestAWiderWindowKeepsTheNarrowerOneAsItsPrefix(t *testing.T) {
	stamp := metav1.NewTime(time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC))
	held := make([]*unstructured.Unstructured, 0, 5)
	for _, name := range []string{"e", "c", "a", "d", "b"} {
		obj := newDeployment("review", name)
		obj.SetCreationTimestamp(stamp)
		held = append(held, obj)
	}

	narrow := newestFirst("Deployment", held, 2)
	wide := newestFirst("Deployment", held, 4)

	for at, obj := range narrow {
		if wide[at].GetName() != obj.GetName() {
			t.Fatalf("position %d = %s in the wide window, %s in the narrow one", at, wide[at].GetName(), obj.GetName())
		}
	}
}

func eventSeenAt(name, seen string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion":    "v1",
		"kind":          "Event",
		"metadata":      map[string]any{"name": name, "namespace": "review", "uid": "uid-" + name},
		"lastTimestamp": seen,
	}}
}

func TestEventsThatWereLastSeenTogetherBreakTheTieByName(t *testing.T) {
	seen := "2026-09-11T10:00:00Z"
	late := eventSeenAt("late", seen)
	early := eventSeenAt("early", seen)

	left := newestFirst(eventKind, []*unstructured.Unstructured{late, early}, 1)
	right := newestFirst(eventKind, []*unstructured.Unstructured{early, late}, 1)

	if left[0].GetName() != "early" || right[0].GetName() != "early" {
		t.Fatalf("windows = %s and %s, want the same event from either order", left[0].GetName(), right[0].GetName())
	}
}
