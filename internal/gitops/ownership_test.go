package gitops

import (
	"slices"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func fieldsEntry(manager, fields string) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{
		Manager:    manager,
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsType: "FieldsV1",
		FieldsV1:   metav1.NewFieldsV1(fields),
	}
}

func ssaObject(entries ...metav1.ManagedFieldsEntry) *unstructured.Unstructured {
	live := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "web", "namespace": "shop"},
		"spec":       map[string]any{"replicas": int64(3)},
	}}
	live.SetManagedFields(entries)
	return live
}

func ownersOf(t *testing.T, live *unstructured.Unstructured, controller string) map[string]string {
	t.Helper()
	found, _ := Ownership(live, controller)
	out := map[string]string{}
	for _, one := range found {
		out[one.Path] = one.Declared + " -> " + one.Live
	}
	return out
}

func TestOwnershipNamesAFieldAnotherManagerTookFromArgo(t *testing.T) {
	live := ssaObject(
		fieldsEntry("argocd-controller", `{"f:spec":{"f:paused":{}}}`),
		fieldsEntry("kubectl-edit", `{"f:spec":{"f:replicas":{}}}`),
	)

	got := ownersOf(t, live, api.ControllerArgo)

	if got["spec.replicas"] != "argocd-controller -> kubectl-edit" {
		t.Fatalf("ownership = %v, want spec.replicas taken by kubectl-edit", got)
	}
	if len(got) != 1 {
		t.Fatalf("ownership = %v, want only what another writer holds", got)
	}
}

func TestOwnershipNamesAFieldTakenFromFlux(t *testing.T) {
	live := ssaObject(
		fieldsEntry("kustomize-controller", `{"f:spec":{"f:replicas":{}}}`),
		fieldsEntry("kubectl-edit", `{"f:spec":{"f:replicas":{}}}`),
	)

	got := ownersOf(t, live, api.ControllerFlux)

	if got["spec.replicas"] != "kustomize-controller -> kubectl-edit" {
		t.Fatalf("ownership = %v", got)
	}
}

func TestOwnershipReadsTheApplicationControllerNameToo(t *testing.T) {
	live := ssaObject(
		fieldsEntry("argocd-application-controller", `{"f:spec":{"f:replicas":{}}}`),
		fieldsEntry("kubectl-edit", `{"f:spec":{"f:replicas":{}}}`),
	)

	if got := ownersOf(t, live, api.ControllerArgo); len(got) != 1 {
		t.Fatalf("ownership = %v, want the statefulset controller understood", got)
	}
}

func TestOwnershipNamesASpecFieldTheControllerDoesNotHoldAtAll(t *testing.T) {
	live := ssaObject(
		fieldsEntry("argocd-controller", `{"f:spec":{"f:replicas":{}}}`),
		fieldsEntry("some-operator", `{"f:spec":{"f:template":{"f:metadata":{"f:labels":{}}}}}`),
	)

	got := ownersOf(t, live, api.ControllerArgo)

	want := "argocd-controller -> some-operator"
	if got["spec.template.metadata.labels"] != want {
		t.Fatalf("ownership = %v, want the field the controller does not hold", got)
	}
}

func TestOwnershipReadsWhatARealApiServerWrote(t *testing.T) {
	argo := `{"f:spec":{"f:selector":{},"f:template":{"f:spec":{"f:containers":` +
		`{"k:{\"name\":\"pause\"}":{".":{},"f:image":{},"f:name":{}}}}}}}`
	live := ssaObject(
		fieldsEntry("argocd-controller", argo),
		fieldsEntry("kubectl-edit", `{"f:spec":{"f:replicas":{}}}`),
	)

	got := ownersOf(t, live, api.ControllerArgo)

	if len(got) != 1 {
		t.Fatalf("ownership = %v, want the one field the other writer took", got)
	}
	if got["spec.replicas"] != "argocd-controller -> kubectl-edit" {
		t.Fatalf("ownership = %v", got)
	}
}

func TestOwnershipSaysSoWhenNothingHasMoved(t *testing.T) {
	live := ssaObject(fieldsEntry("argocd-controller", `{"f:spec":{"f:replicas":{}}}`))

	found, note := Ownership(live, api.ControllerArgo)

	if found != nil {
		t.Fatalf("ownership = %v, want none", found)
	}
	if !strings.HasPrefix(note, heldNote) {
		t.Fatalf("note = %q, want it to say nothing else holds a spec field", note)
	}
	if !strings.HasSuffix(note, "argocd-controller") {
		t.Fatalf("note = %q, want the manager named", note)
	}
}

func TestOwnershipFallsBackToTheOldNoteWhenNoGitopsManagerIsThere(t *testing.T) {
	live := ssaObject(fieldsEntry("kubectl-client-side-apply", `{"f:spec":{"f:replicas":{}}}`))

	found, note := Ownership(live, api.ControllerArgo)

	if found != nil {
		t.Fatalf("ownership = %v, want none", found)
	}
	if note != noDeclaration {
		t.Fatalf("note = %q, want the note that says there is nothing to compare", note)
	}
}

func TestOwnershipIgnoresAStatusWriter(t *testing.T) {
	entries := []metav1.ManagedFieldsEntry{
		fieldsEntry("argocd-controller", `{"f:spec":{"f:replicas":{}}}`),
		fieldsEntry("kube-controller-manager", `{"f:spec":{"f:replicas":{}}}`),
	}
	entries[1].Subresource = "status"
	live := ssaObject(entries...)

	if got := ownersOf(t, live, api.ControllerArgo); len(got) != 0 {
		t.Fatalf("ownership = %v, want a status writer left out of it", got)
	}
}

func TestOwnershipSkipsTheControllersOwnStatusEntry(t *testing.T) {
	entries := []metav1.ManagedFieldsEntry{
		fieldsEntry("argocd-controller", `{"f:status":{"f:replicas":{}}}`),
		fieldsEntry("argocd-controller", `{"f:spec":{"f:paused":{}}}`),
		fieldsEntry("kubectl-edit", `{"f:spec":{"f:replicas":{}}}`),
	}
	entries[0].Subresource = "status"
	live := ssaObject(entries...)

	if got := ownersOf(t, live, api.ControllerArgo); len(got) != 1 {
		t.Fatalf("ownership = %v, want the spec entry used, not the status one", got)
	}
}

func TestOwnershipCountsAFieldOnceWhenTwoWritersHoldIt(t *testing.T) {
	live := ssaObject(
		fieldsEntry("argocd-controller", `{"f:spec":{"f:paused":{}}}`),
		fieldsEntry("kubectl-edit", `{"f:spec":{"f:replicas":{}}}`),
		fieldsEntry("some-operator", `{"f:spec":{"f:replicas":{}}}`),
	)

	found, _ := Ownership(live, api.ControllerArgo)

	if len(found) != 1 {
		t.Fatalf("ownership = %+v, want one row for one field", found)
	}
	if found[0].Live != "kubectl-edit" {
		t.Fatalf("taken by = %q, want the first writer that holds it", found[0].Live)
	}
}

func TestFieldPathsReadTheShapesTheApiServerWrites(t *testing.T) {
	cases := []struct {
		name   string
		fields string
		want   string
	}{
		{
			name:   "a plain field",
			fields: `{"f:spec":{"f:replicas":{}}}`,
			want:   "spec.replicas",
		},
		{
			name:   "a nested field",
			fields: `{"f:spec":{"f:template":{"f:spec":{"f:serviceAccountName":{}}}}}`,
			want:   "spec.template.spec.serviceAccountName",
		},
		{
			name:   "a list entry keyed by name",
			fields: `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"web\"}":{"f:image":{}}}}}}}`,
			want:   "spec.template.spec.containers[web].image",
		},
		{
			name:   "a list entry keyed by something else",
			fields: `{"f:spec":{"f:ports":{"k:{\"port\":80,\"protocol\":\"TCP\"}":{"f:targetPort":{}}}}}`,
			want:   "spec.ports[port=80,protocol=TCP].targetPort",
		},
		{
			name:   "a set entry",
			fields: `{"f:spec":{"f:finalizers":{"v:\"keep\"":{}}}}`,
			want:   "spec.finalizers[keep]",
		},
		{
			name:   "an indexed entry",
			fields: `{"f:spec":{"f:rules":{"i:0":{}}}}`,
			want:   "spec.rules[0]",
		},
		{
			name:   "a key the api server did not prefix",
			fields: `{"f:spec":{"odd":{}}}`,
			want:   "spec.odd",
		},
		{
			name:   "a malformed field value",
			fields: `{"f:spec":{"f:replicas":true}}`,
			want:   "spec.replicas",
		},
		{
			name:   "a key that is not readable json",
			fields: `{"f:spec":{"f:ports":{"k:not-json":{}}}}`,
			want:   "spec.ports[not-json]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := fieldsEntry("argocd-controller", tc.fields)

			paths := specPathsOf(&entry)

			if !paths[tc.want] {
				t.Fatalf("paths = %v, want %q among them", sorted(paths), tc.want)
			}
		})
	}
}

func TestFieldPathsSurviveAnEntryThatSaysNothing(t *testing.T) {
	cases := []struct {
		name  string
		entry metav1.ManagedFieldsEntry
	}{
		{name: "no fields at all", entry: metav1.ManagedFieldsEntry{Manager: "argocd-controller"}},
		{name: "fields that are not json", entry: fieldsEntry("argocd-controller", `not json`)},
		{name: "no spec in them", entry: fieldsEntry("argocd-controller", `{"f:metadata":{"f:labels":{}}}`)},
		{name: "a spec that is not a map", entry: fieldsEntry("argocd-controller", `{"f:spec":"nope"}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if paths := specPathsOf(&tc.entry); len(paths) != 0 {
				t.Fatalf("paths = %v, want none", sorted(paths))
			}
		})
	}
}

func TestOwnershipStopsAfterTwentyFieldsAndCountsTheRest(t *testing.T) {
	owned := map[string]any{}
	for at := range 25 {
		owned["f:field"+string(rune('a'+at))] = map[string]any{}
	}
	encoded := `{"f:spec":{`
	parts := make([]string, 0, len(owned))
	for key := range owned {
		parts = append(parts, `"`+key+`":{}`)
	}
	slices.Sort(parts)
	encoded += strings.Join(parts, ",") + `}}`
	live := ssaObject(fieldsEntry("argocd-controller", encoded), fieldsEntry("kubectl-edit", encoded))

	found, note := Ownership(live, api.ControllerArgo)

	if len(found) != maxDriftFields {
		t.Fatalf("ownership = %d rows, want %d", len(found), maxDriftFields)
	}
	if !strings.HasSuffix(note, "and 5 more") {
		t.Fatalf("note = %q, want the rest counted", note)
	}
}
