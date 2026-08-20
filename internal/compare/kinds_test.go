package compare

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func deploy(namespace, name string, replicas int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":            name,
			"namespace":       namespace,
			"uid":             "uid-" + namespace + "-" + name,
			"resourceVersion": "1",
		},
		"spec":   map[string]any{"replicas": replicas},
		"status": map[string]any{"readyReplicas": replicas},
	}}
}

func verdicts(objects []api.KindDiff) map[string]string {
	out := map[string]string{}
	for _, object := range objects {
		out[object.Namespace+"/"+object.Name] = object.Verdict
	}
	return out
}

func TestObjectsOnBothSidesAreComparedNotJustCounted(t *testing.T) {
	left := []*unstructured.Unstructured{
		deploy("prod", "web", 2),
		deploy("prod", "api", 3),
	}
	right := []*unstructured.Unstructured{
		deploy("prod", "web", 2),
		deploy("prod", "api", 13),
	}

	found := Kinds(left, right, false)

	got := verdicts(found)
	if got["prod/web"] != api.VerdictSame {
		t.Fatalf("web = %q, want same", got["prod/web"])
	}
	if got["prod/api"] != api.VerdictDiffers {
		t.Fatalf("api = %q, want differs", got["prod/api"])
	}
}

func TestTheServerSideFieldsDoNotCountAsDrift(t *testing.T) {
	here := deploy("prod", "web", 2)
	there := deploy("prod", "web", 2)
	there.SetUID("a-different-uid")
	there.SetResourceVersion("9999")
	there.SetCreationTimestamp(metav1.NewTime(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)))
	_ = unstructured.SetNestedField(there.Object, int64(7), "status", "readyReplicas")

	found := Kinds([]*unstructured.Unstructured{here}, []*unstructured.Unstructured{there}, false)

	if found[0].Verdict != api.VerdictSame {
		t.Fatalf("verdict = %q, want same: only what a person wrote should count", found[0].Verdict)
	}
}

func TestAnObjectOnlyOneSideHasIsNamedAsSuch(t *testing.T) {
	left := []*unstructured.Unstructured{deploy("prod", "web", 2), deploy("prod", "here-only", 1)}
	right := []*unstructured.Unstructured{deploy("prod", "web", 2), deploy("prod", "there-only", 1)}

	found := Kinds(left, right, false)

	got := verdicts(found)
	if got["prod/here-only"] != api.VerdictOnlyHere {
		t.Fatalf("here-only = %q", got["prod/here-only"])
	}
	if got["prod/there-only"] != api.VerdictOnlyThere {
		t.Fatalf("there-only = %q", got["prod/there-only"])
	}
	if len(found) != 3 {
		t.Fatalf("objects = %d, want both sides' names once each", len(found))
	}
}

func TestTheSameNameInTwoNamespacesIsTwoObjects(t *testing.T) {
	left := []*unstructured.Unstructured{deploy("prod", "web", 2)}
	right := []*unstructured.Unstructured{deploy("staging", "web", 2)}

	found := Kinds(left, right, false)

	if len(found) != 2 {
		t.Fatalf("objects = %+v, want the namespace to keep them apart", found)
	}
}

func TestNamespacesOfTheirOwnAreMatchedByNameAlone(t *testing.T) {
	left := []*unstructured.Unstructured{deploy("prod", "web", 2)}
	right := []*unstructured.Unstructured{deploy("staging", "web", 2)}

	found := Kinds(left, right, true)

	if len(found) != 1 {
		t.Fatalf("objects = %+v, want one pair", found)
	}
	if found[0].Verdict != api.VerdictDiffers {
		t.Fatalf("verdict = %q, want differs: the namespace itself is a difference", found[0].Verdict)
	}
	if found[0].Lines == 0 {
		t.Fatal("a differing pair counted no lines")
	}
}

func TestTheLineCountMatchesWhatChanged(t *testing.T) {
	left := []*unstructured.Unstructured{deploy("prod", "web", 2)}
	right := []*unstructured.Unstructured{deploy("prod", "web", 3)}

	found := Kinds(left, right, false)

	if found[0].Lines != 2 {
		t.Fatalf("lines = %d, want the replicas line on each side", found[0].Lines)
	}
}

func TestObjectsComeBackInAStableOrder(t *testing.T) {
	left := []*unstructured.Unstructured{
		deploy("prod", "web", 1),
		deploy("apps", "zeta", 1),
		deploy("apps", "alpha", 1),
	}

	found := Kinds(left, nil, false)

	order := make([]string, 0, len(found))
	for _, object := range found {
		order = append(order, object.Namespace+"/"+object.Name)
	}
	want := []string{"apps/alpha", "apps/zeta", "prod/web"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestTwoEmptySidesCompareToNothing(t *testing.T) {
	found := Kinds(nil, nil, false)

	if len(found) != 0 {
		t.Fatalf("objects = %+v, want none", found)
	}
	same, differs, onlyHere, onlyThere := Tally(found)
	if same+differs+onlyHere+onlyThere != 0 {
		t.Fatalf("tally = %d %d %d %d", same, differs, onlyHere, onlyThere)
	}
}

func TestTheTallyCountsEachVerdict(t *testing.T) {
	objects := []api.KindDiff{
		{Verdict: api.VerdictSame},
		{Verdict: api.VerdictSame},
		{Verdict: api.VerdictDiffers},
		{Verdict: api.VerdictOnlyHere},
		{Verdict: api.VerdictOnlyThere},
		{Verdict: "something else"},
	}

	same, differs, onlyHere, onlyThere := Tally(objects)

	if same != 2 || differs != 1 || onlyHere != 1 || onlyThere != 1 {
		t.Fatalf("tally = %d %d %d %d", same, differs, onlyHere, onlyThere)
	}
}

func TestClusterScopedObjectsCompareWithoutANamespace(t *testing.T) {
	here := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata":   map[string]any{"name": "cluster-admin"},
		"rules":      []any{map[string]any{"verbs": []any{"*"}}},
	}}
	there := here.DeepCopy()

	found := Kinds([]*unstructured.Unstructured{here}, []*unstructured.Unstructured{there}, false)

	if len(found) != 1 || found[0].Verdict != api.VerdictSame {
		t.Fatalf("found = %+v, want one identical cluster-scoped object", found)
	}
	if found[0].Namespace != "" {
		t.Fatalf("namespace = %q, want none", found[0].Namespace)
	}
}
