package resources

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

// crdWith builds a resource definition the way the apiserver hands one back,
// with the columns its author asked to be shown by.
func crdWith(version string, columns ...map[string]any) *unstructured.Unstructured {
	entries := make([]any, 0, len(columns))
	for _, column := range columns {
		entries = append(entries, column)
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": "kustomizations.kustomize.toolkit.fluxcd.io"},
		"spec": map[string]any{
			"group": "kustomize.toolkit.fluxcd.io",
			"versions": []any{map[string]any{
				"name":                     version,
				"additionalPrinterColumns": entries,
			}},
		},
	}}
}

func column(name, kind, path string) map[string]any {
	return map[string]any{"name": name, "type": kind, "jsonPath": path}
}

func kustomization() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]any{
			"name":              "apps",
			"namespace":         "flux-system",
			"creationTimestamp": "2026-08-01T09:00:00Z",
		},
		"spec": map[string]any{"suspend": false, "path": "./clusters/p-mk1"},
		"status": map[string]any{
			"lastAppliedRevision": "main@sha1:abc123",
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True", "message": "Applied revision"},
			},
		},
	}}
}

func columnNames(columns []api.Column) []string {
	out := make([]string, 0, len(columns))
	for _, one := range columns {
		out = append(out, one.Name)
	}
	return out
}

func TestAResourceIsShownTheColumnsItsDefinitionAsksFor(t *testing.T) {
	crd := crdWith(
		"v1",
		column("Ready", "string", `.status.conditions[?(@.type=="Ready")].status`),
		column("Revision", "string", ".status.lastAppliedRevision"),
	)

	shown, ok := layoutOf(crd, "v1")

	if !ok {
		t.Fatal("a definition with columns was read as having none")
	}
	if got := columnNames(shown.columns); strings.Join(got, ",") != "Ready,Revision" {
		t.Fatalf("columns = %v, want the ones the definition asks for", got)
	}
	cells := shown.cells(kustomization())
	if strings.Join(cells, "|") != "True|main@sha1:abc123" {
		t.Fatalf("cells = %v, want the values those paths point at", cells)
	}
}

// A path into a field the object does not have is not an error: half the
// columns on a custom resource are empty until something fills them in.
func TestAColumnPointingAtNothingIsEmpty(t *testing.T) {
	crd := crdWith("v1", column("Message", "string", ".status.nothing.here"))

	shown, ok := layoutOf(crd, "v1")

	if !ok {
		t.Fatal("a definition with columns was read as having none")
	}
	if cells := shown.cells(kustomization()); strings.Join(cells, "") != "" {
		t.Fatalf("cells = %v, want a blank cell", cells)
	}
}

func TestAPathThatFindsSeveralValuesJoinsThem(t *testing.T) {
	crd := crdWith("v1", column("Types", "string", ".status.conditions[*].type"))
	object := kustomization()
	conditions := []any{
		map[string]any{"type": "Ready", "status": "True"},
		map[string]any{"type": "Healthy", "status": "True"},
	}
	_ = unstructured.SetNestedSlice(object.Object, conditions, "status", "conditions")

	shown, _ := layoutOf(crd, "v1")

	if cells := shown.cells(object); cells[0] != "Ready, Healthy" {
		t.Fatalf("cell = %q, want every value the path found", cells[0])
	}
}

// kubectl keeps these back until asked for a wide table, and so does spinoza:
// they are the ones the author thought were too much for a first look.
func TestAColumnMarkedWideIsLeftOut(t *testing.T) {
	wide := column("Path", "string", ".spec.path")
	wide["priority"] = int64(1)
	crd := crdWith("v1", column("Ready", "string", `.status.conditions[?(@.type=="Ready")].status`), wide)

	shown, ok := layoutOf(crd, "v1")

	if !ok {
		t.Fatal("a definition with columns was read as having none")
	}
	if got := columnNames(shown.columns); strings.Join(got, ",") != "Ready" {
		t.Fatalf("columns = %v, want the wide one left out", got)
	}
}

func TestAWidePriorityWrittenAsANumberIsLeftOutToo(t *testing.T) {
	wide := column("Path", "string", ".spec.path")
	wide["priority"] = float64(1)
	crd := crdWith("v1", wide)

	_, ok := layoutOf(crd, "v1")

	if ok {
		t.Fatal("a definition whose only column is wide was read as having one")
	}
}

// Every table already has a name and an age. Showing them again because the
// definition mentions them would be two of each.
func TestTheColumnsEveryTableAlreadyHasAreLeftOut(t *testing.T) {
	crd := crdWith(
		"v1",
		column("Name", "string", ".metadata.name"),
		column("Age", "date", ".metadata.creationTimestamp"),
		column("Ready", "string", `.status.conditions[?(@.type=="Ready")].status`),
	)

	shown, ok := layoutOf(crd, "v1")

	if !ok {
		t.Fatal("a definition with columns was read as having none")
	}
	if got := columnNames(shown.columns); strings.Join(got, ",") != "Ready" {
		t.Fatalf("columns = %v, want only the one the table does not already have", got)
	}
}

func TestADefinitionOfNothingButTheUsualColumnsIsNotUsed(t *testing.T) {
	crd := crdWith("v1", column("Age", "date", ".metadata.creationTimestamp"))

	_, ok := layoutOf(crd, "v1")

	if ok {
		t.Fatal("a definition with nothing left to show was used anyway")
	}
}

// A date is drawn the way every other age in spinoza is drawn, rather than as a
// timestamp nobody reads.
func TestADateColumnIsDrawnAsAnAge(t *testing.T) {
	crd := crdWith("v1", column("Last run", "date", ".status.lastHandledAt"))

	shown, ok := layoutOf(crd, "v1")

	if !ok {
		t.Fatal("a definition with columns was read as having none")
	}
	if shown.columns[0].Render != "age" {
		t.Fatalf("render = %q, want an age", shown.columns[0].Render)
	}
}

func TestOtherColumnsAreDrawnPlainly(t *testing.T) {
	crd := crdWith("v1", column("Replicas", "integer", ".spec.replicas"))

	shown, _ := layoutOf(crd, "v1")

	if shown.columns[0].Render != "" {
		t.Fatalf("render = %q, want nothing special", shown.columns[0].Render)
	}
}

// A definition serves several versions and they need not agree, so the answer
// has to be about the version being listed.
func TestTheColumnsAreTakenFromTheVersionBeingListed(t *testing.T) {
	crd := crdWith("v1beta2", column("Ready", "string", ".status.ready"))

	_, ok := layoutOf(crd, "v1")

	if ok {
		t.Fatal("columns for another version were used")
	}
}

func TestADefinitionThatAsksForNothingIsNotUsed(t *testing.T) {
	crd := crdWith("v1")

	_, ok := layoutOf(crd, "v1")

	if ok {
		t.Fatal("a definition with no columns was used")
	}
}

func TestADefinitionWithNoVersionsAtAllIsNotUsed(t *testing.T) {
	crd := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{}}}

	_, ok := layoutOf(crd, "v1")

	if ok {
		t.Fatal("a definition with no versions was used")
	}
}

func TestADefinitionThatIsNotShapedLikeOneIsNotUsed(t *testing.T) {
	crd := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"versions": "not a list"},
	}}

	_, ok := layoutOf(crd, "v1")

	if ok {
		t.Fatal("a definition that could not be read was used")
	}
}

func TestAVersionEntryThatIsNotShapedLikeOneIsSkipped(t *testing.T) {
	crd := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"versions": []any{"not an object"}},
	}}

	_, ok := layoutOf(crd, "v1")

	if ok {
		t.Fatal("a version that could not be read was used")
	}
}

func TestAVersionWhoseColumnsAreNotAListIsNotUsed(t *testing.T) {
	crd := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"versions": []any{map[string]any{
			"name":                     "v1",
			"additionalPrinterColumns": "not a list",
		}}},
	}}

	_, ok := layoutOf(crd, "v1")

	if ok {
		t.Fatal("columns that could not be read were used")
	}
}

func TestAColumnMissingWhatItNeedsIsSkipped(t *testing.T) {
	cases := []struct {
		what  string
		entry any
	}{
		{"not an object", "a string"},
		{"no name", map[string]any{"type": "string", "jsonPath": ".spec.path"}},
		{"no path", map[string]any{"name": "Path", "type": "string"}},
		{"a path that will not parse", column("Path", "string", ".spec[")},
	}
	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			crd := crdWith("v1")
			_ = unstructured.SetNestedSlice(
				crd.Object,
				[]any{map[string]any{"name": "v1", "additionalPrinterColumns": []any{tc.entry}}},
				"spec", "versions",
			)

			_, ok := layoutOf(crd, "v1")

			if ok {
				t.Fatalf("a column with %s was used", tc.what)
			}
		})
	}
}

// A path may be written either way round, and definitions in the wild use both.
func TestAPathAlreadyInBracesIsReadTheSame(t *testing.T) {
	plain := crdWith("v1", column("Revision", "string", ".status.lastAppliedRevision"))
	braced := crdWith("v1", column("Revision", "string", "{.status.lastAppliedRevision}"))

	fromPlain, _ := layoutOf(plain, "v1")
	fromBraced, _ := layoutOf(braced, "v1")

	object := kustomization()
	if fromPlain.cells(object)[0] != fromBraced.cells(object)[0] {
		t.Fatalf("%v and %v, want the same value", fromPlain.cells(object), fromBraced.cells(object))
	}
}

// Rows are built by the informer and by whoever is taking a snapshot, so every
// reader has to get the same answer.
func TestAColumnCanBeReadFromSeveralGoroutinesAtOnce(t *testing.T) {
	crd := crdWith("v1", column("Ready", "string", `.status.conditions[?(@.type=="Ready")].status`))
	shown, _ := layoutOf(crd, "v1")
	object := kustomization()

	var group sync.WaitGroup
	for range 50 {
		group.Go(func() {
			if got := shown.cells(object); got[0] != "True" {
				t.Errorf("cell = %q, want the condition's status", got[0])
			}
		})
	}
	group.Wait()
}

var (
	kustomizationGVR = schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	kustomizationKey = discovery.Key("kustomize.toolkit.fluxcd.io", "v1", "kustomizations")
)

func kustomizationDesc() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      "kustomize.toolkit.fluxcd.io",
		Version:    "v1",
		Resource:   "kustomizations",
		Kind:       "Kustomization",
		Namespaced: true,
		Category:   "Flux",
	}
}

// crdServing builds a cluster that holds one custom resource and the definition
// behind it, which is what a real cluster with flux on it looks like.
func crdServing(t *testing.T, definition *unstructured.Unstructured) (*Manager, context.CancelFunc) {
	t.Helper()
	kinds := listKinds()
	kinds[kustomizationGVR] = "KustomizationList"
	kinds[crdGVR] = "CustomResourceDefinitionList"
	objects := []runtime.Object{kustomization()}
	if definition != nil {
		objects = append(objects, definition)
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objects...)
	ctx, cancel := context.WithCancel(context.Background())
	descs := testDescs()
	descs[kustomizationKey] = kustomizationDesc()
	mgr := NewManager(ctx, Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Categories:  []api.Category{{Name: "Flux"}},
		Descriptors: descs,
		Limits:      Limits{IdleGrace: time.Millisecond},
	})
	return mgr, cancel
}

func subscribeToKustomizations(t *testing.T, mgr *Manager) *Subscription {
	t.Helper()
	sub, err := mgr.Subscribe(
		t.Context(),
		"kustomize.toolkit.fluxcd.io", "v1", "kustomizations", "", 0, nil,
	)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	return sub
}

// The whole point: a kind spinoza has no table for is shown the way its own
// definition asks, rather than as a single status.
func TestAKindSpinozaDoesNotKnowIsShownAsItsDefinitionAsks(t *testing.T) {
	crd := crdWith(
		"v1",
		column("Ready", "string", `.status.conditions[?(@.type=="Ready")].status`),
		column("Revision", "string", ".status.lastAppliedRevision"),
	)
	mgr, cancel := crdServing(t, crd)
	defer cancel()

	sub := subscribeToKustomizations(t, mgr)

	if got := columnNames(sub.Columns); strings.Join(got, ",") != "Ready,Revision" {
		t.Fatalf("columns = %v, want the ones the definition asks for", got)
	}
	if len(sub.Rows) != 1 {
		t.Fatalf("rows = %d, want the one object", len(sub.Rows))
	}
	if strings.Join(sub.Rows[0].Cells, "|") != "True|main@sha1:abc123" {
		t.Fatalf("cells = %v, want what the definition points at", sub.Rows[0].Cells)
	}
}

// A cluster whose definitions cannot be read is the common one: plenty of users
// have no rights on them at all.
func TestAKindWhoseDefinitionCannotBeReadIsShownAsBefore(t *testing.T) {
	mgr, cancel := crdServing(t, nil)
	defer cancel()

	sub := subscribeToKustomizations(t, mgr)

	if got := columnNames(sub.Columns); strings.Join(got, ",") != "Status" {
		t.Fatalf("columns = %v, want the single status it has always been", got)
	}
	if len(sub.Rows) != 1 {
		t.Fatalf("rows = %d, want the object listed anyway", len(sub.Rows))
	}
}

// Spinoza's own tables say more than a definition can, and they are kept.
func TestAKindSpinozaKnowsKeepsItsOwnTable(t *testing.T) {
	crd := crdWith("v1", column("Nonsense", "string", ".spec.nonsense"))
	mgr, cancel := crdServing(t, crd)
	defer cancel()

	sub, err := mgr.Subscribe(t.Context(), "apps", "v1", "deployments", "", 0, nil)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)

	if got := columnNames(sub.Columns); strings.Join(got, ",") != "Ready,Up-to-date,Available" {
		t.Fatalf("columns = %v, want spinoza's own", got)
	}
}
