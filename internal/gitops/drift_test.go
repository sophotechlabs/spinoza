package gitops

import (
	"maps"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func declaring(declared string, live map[string]any) *unstructured.Unstructured {
	object := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":        "web",
			"namespace":   "shop",
			"annotations": map[string]any{lastAppliedAnnotation: declared},
		},
	}
	maps.Copy(object, live)
	return &unstructured.Unstructured{Object: object}
}

func pathsOf(t *testing.T, live *unstructured.Unstructured) map[string]string {
	t.Helper()
	found, note := Drift(live)
	if note != "" {
		t.Fatalf("note = %q, want none", note)
	}
	out := map[string]string{}
	for _, one := range found {
		out[one.Path] = one.Declared + " -> " + one.Live
	}
	return out
}

func TestDriftNamesTheFieldThatMoved(t *testing.T) {
	live := declaring(`{"spec":{"replicas":1}}`, map[string]any{
		"spec": map[string]any{"replicas": int64(3)},
	})

	got := pathsOf(t, live)

	if got["spec.replicas"] != "1 -> 3" {
		t.Fatalf("drift = %v, want spec.replicas 1 -> 3", got)
	}
	if len(got) != 1 {
		t.Fatalf("drift = %v, want only the field that moved", got)
	}
}

func TestDriftIgnoresDefaultsTheServerAdded(t *testing.T) {
	live := declaring(`{"spec":{"ports":[{"port":80,"targetPort":80}]}}`, map[string]any{
		"spec": map[string]any{
			"ports": []any{map[string]any{"port": int64(80), "targetPort": int64(80), "protocol": "TCP"}},
		},
	})

	if got := pathsOf(t, live); len(got) != 0 {
		t.Fatalf("drift = %v, want nothing: the declaration never named protocol", got)
	}
}

func TestDriftReadsIntoListEntries(t *testing.T) {
	live := declaring(`{"spec":{"containers":[{"name":"web","image":"web:1"}]}}`, map[string]any{
		"spec": map[string]any{
			"containers": []any{map[string]any{"name": "web", "image": "web:2"}},
		},
	})

	got := pathsOf(t, live)

	if got["spec.containers[0].image"] != "web:1 -> web:2" {
		t.Fatalf("drift = %v, want the image inside the list", got)
	}
}

func TestDriftReportsAListThatChangedLength(t *testing.T) {
	live := declaring(`{"spec":{"ports":[{"port":80},{"port":443}]}}`, map[string]any{
		"spec": map[string]any{"ports": []any{map[string]any{"port": int64(80)}}},
	})

	got := pathsOf(t, live)

	if got["spec.ports"] != "2 entries -> 1 entry" {
		t.Fatalf("drift = %v, want the count difference", got)
	}
}

func TestDriftReportsAFieldThatIsGone(t *testing.T) {
	live := declaring(`{"spec":{"replicas":2}}`, map[string]any{"spec": map[string]any{}})

	got := pathsOf(t, live)

	if got["spec.replicas"] != "2 -> not set" {
		t.Fatalf("drift = %v", got)
	}
}

func TestDriftComparesEveryScalarShape(t *testing.T) {
	live := declaring(
		`{"spec":{"paused":false,"replicas":1,"ratio":0.5,"name":"web","empty":null,"list":["a"]}}`,
		map[string]any{"spec": map[string]any{
			"paused":   true,
			"replicas": int64(2),
			"ratio":    0.75,
			"name":     "api",
			"empty":    "set now",
			"list":     []any{"b"},
		}},
	)

	got := pathsOf(t, live)

	want := map[string]string{
		"spec.paused":   "false -> true",
		"spec.replicas": "1 -> 2",
		"spec.ratio":    "0.5 -> 0.75",
		"spec.name":     "web -> api",
		"spec.empty":    "not set -> set now",
		"spec.list[0]":  "a -> b",
	}
	for path, expected := range want {
		if got[path] != expected {
			t.Fatalf("%s = %q, want %q", path, got[path], expected)
		}
	}
}

func TestDriftIgnoresTheFieldsEveryDeclarationCarries(t *testing.T) {
	live := declaring(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"shop","creationTimestamp":null}}`,
		map[string]any{})

	if got := pathsOf(t, live); len(got) != 0 {
		t.Fatalf("drift = %v, want nothing", got)
	}
}

func TestDriftSaysSoWhenThereIsNoDeclarationToCompare(t *testing.T) {
	live := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "web"},
	}}

	found, note := Drift(live)

	if found != nil {
		t.Fatalf("drift = %v, want none", found)
	}
	if !strings.Contains(note, "ServerSideApply") {
		t.Fatalf("note = %q, want it to name the case that has no annotation", note)
	}
}

func TestDriftSaysSoWhenTheDeclarationIsEmpty(t *testing.T) {
	live := declaring("", map[string]any{})

	_, note := Drift(live)

	if note == "" {
		t.Fatal("an empty declaration was read as no drift")
	}
}

func TestDriftSaysSoWhenTheDeclarationIsNotReadable(t *testing.T) {
	live := declaring("{not json", map[string]any{})

	_, note := Drift(live)

	if note == "" {
		t.Fatal("an unreadable declaration was read as no drift")
	}
}

func TestDriftStopsAfterTwentyFieldsAndCountsTheRest(t *testing.T) {
	declared := map[string]any{}
	liveSpec := map[string]any{}
	for at := range 25 {
		key := string(rune('a'+at/5)) + string(rune('a'+at%5))
		declared[key] = "declared"
		liveSpec[key] = "live"
	}
	encoded := `{"spec":{`
	parts := make([]string, 0, len(declared))
	for key := range declared {
		parts = append(parts, `"`+key+`":"declared"`)
	}
	encoded += strings.Join(parts, ",") + `}}`
	live := declaring(encoded, map[string]any{"spec": liveSpec})

	found, note := Drift(live)

	if len(found) != maxDriftFields {
		t.Fatalf("drift = %d fields, want %d", len(found), maxDriftFields)
	}
	if note != "5 more fields differ" {
		t.Fatalf("note = %q, want the rest counted", note)
	}
}
