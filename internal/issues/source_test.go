package issues

import (
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestAListThatFailsIsReportedOnTheQueue(t *testing.T) {
	lister := &stubLister{errs: map[string]error{"pods": errors.New("forbidden")}}

	queue := build(t, lister, catalog(podDescriptor()))

	want := "1 of 1 resource types could not be listed: pods (forbidden)"
	if queue.Error != want {
		t.Fatalf("error = %q, want %q", queue.Error, want)
	}
}

func TestOnlyTheTypesTheEngineNeedsAreRead(t *testing.T) {
	lister := &stubLister{}
	descs := catalog(
		podDescriptor(),
		deploymentDescriptor(),
		descriptor("", "v1", "configmaps", "ConfigMap"),
	)

	build(t, lister, descs)

	for _, desc := range lister.leased {
		if desc.Resource == "configmaps" {
			t.Fatalf("leased = %+v, want config maps left alone", lister.leased)
		}
	}
	if len(lister.leased) != 2 {
		t.Fatalf("leased = %+v, want the pod and deployment types", lister.leased)
	}
}

func TestAFluxResourceOutsideTheListIsNotCollected(t *testing.T) {
	receivers := descriptor("notification.toolkit.fluxcd.io", "v1", "receivers", "Receiver")

	if collected(receivers) {
		t.Fatal("collected = true, want receivers left out")
	}
}

func TestAFluxSourceIsCollected(t *testing.T) {
	repos := descriptor("source.toolkit.fluxcd.io", "v1", "gitrepositories", "GitRepository")

	if !collected(repos) {
		t.Fatal("collected = false, want git repositories in")
	}
}

func TestAGroupThatMerelyEndsSimilarlyIsNotFlux(t *testing.T) {
	if isFluxGroup(".toolkit.fluxcd.io") {
		t.Fatal("isFluxGroup = true, want the bare suffix rejected")
	}
	if isFluxGroup("example.com") {
		t.Fatal("isFluxGroup = true, want an unrelated group rejected")
	}
}

func TestAnOwnerCycleStopsAtTheDepthLimit(t *testing.T) {
	first := newWorkload(kindReplicaSet, "one", "uid-one", map[string]any{}, map[string]any{})
	second := newWorkload(kindReplicaSet, "two", "uid-two", map[string]any{}, map[string]any{})
	controller := true
	first.SetOwnerReferences(ownerReference(kindReplicaSet, "two", "uid-two", &controller))
	second.SetOwnerReferences(ownerReference(kindReplicaSet, "one", "uid-one", &controller))
	snap := newSnapshot()
	desc := replicaSetDescriptor()
	for _, obj := range []*unstructured.Unstructured{first, second} {
		entry := object{obj: obj, desc: desc}
		snap.byUID[entry.uid()] = entry
	}

	owner := snap.owner(snap.byUID["uid-one"])

	if owner.uid() != "uid-one" && owner.uid() != "uid-two" {
		t.Fatalf("owner = %q, want the walk to stop inside the cycle", owner.uid())
	}
}

func TestAnObjectWithNoControllerOwnerIsItsOwnOwner(t *testing.T) {
	obj := newWorkload(kindReplicaSet, "one", "uid-one", map[string]any{}, map[string]any{})
	obj.SetOwnerReferences(ownerReference(kindDeployment, "web", "uid-web", nil))
	entry := object{obj: obj, desc: replicaSetDescriptor()}
	snap := &snapshot{byUID: map[string]object{"uid-one": entry}, byKind: map[string][]object{}}

	if got := snap.owner(entry); got.uid() != "uid-one" {
		t.Fatalf("owner = %q, want the object itself", got.uid())
	}
}

func TestACachedTypeIsRead(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{
		"conditions": []any{condition("Ready", "False", nil)},
	})
	lister := &stubLister{
		items:  cachedItems(obj),
		cached: []api.ResourceDescriptor{certificateDescriptor()},
	}

	if queue := build(t, lister, catalog()); len(queue.Rows) != 1 {
		t.Fatalf("rows = %+v, want the cached type read", queue.Rows)
	}
}

func TestARefFromAnObjectCarriesItsCoordinates(t *testing.T) {
	entry := object{obj: newPod("web-1"), desc: podDescriptor()}

	ref := entry.ref()

	want := api.ObjectRef{Version: "v1", Resource: "pods", Namespace: "default", Name: "web-1"}
	if ref != want {
		t.Fatalf("ref = %+v, want %+v", ref, want)
	}
}

func TestTheKindIndexIsKeyedByGroup(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{}, map[string]any{})
	lister := &stubLister{items: deploymentItems(deployment)}
	snap := collect(t.Context(), lister, catalog(deploymentDescriptor()), testLimits().orDefaults())

	if len(snap.of(appsGroup, kindDeployment)) != 1 {
		t.Fatalf("index = %+v, want the deployment under its group", snap.byKind)
	}
	if len(snap.of("", kindDeployment)) != 0 {
		t.Fatalf("index = %+v, want nothing under the core group", snap.byKind)
	}
}

func TestBuildAsksForTheTimeOnce(t *testing.T) {
	calls := 0
	lister := &stubLister{}

	Build(t.Context(), lister, &stubEvents{}, catalog(podDescriptor()), func() time.Time {
		calls++
		return testNow
	}, testLimits())

	if calls != 1 {
		t.Fatalf("clock reads = %d, want one so every detector judges the same moment", calls)
	}
}

func TestOneForbiddenTypeDoesNotSilenceTheRest(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(2)})
	lister := &stubLister{
		items: map[string][]*unstructured.Unstructured{"deployments": {deployment}},
		errs:  map[string]error{"pods": errors.New("pods is forbidden")},
	}

	queue := build(t, lister, catalog(podDescriptor(), deploymentDescriptor()))

	if len(queue.Rows) != 1 {
		t.Fatalf("rows = %+v, want the deployment still judged when pods are refused", queue.Rows)
	}
	if queue.Error == "" {
		t.Fatalf("error = %q, want the refusal still reported alongside the rows", queue.Error)
	}
}
