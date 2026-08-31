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

	if got["spec.containers[web].image"] != "web:1 -> web:2" {
		t.Fatalf("drift = %v, want the image inside the named entry", got)
	}
}

func TestDriftFollowsANamedEntryThatMoved(t *testing.T) {
	live := declaring(
		`{"spec":{"containers":[{"name":"web","image":"web:1"},{"name":"sidecar","image":"log:1"}]}}`,
		map[string]any{"spec": map[string]any{"containers": []any{
			map[string]any{"name": "sidecar", "image": "log:1"},
			map[string]any{"name": "web", "image": "web:1"},
		}}},
	)

	if got := pathsOf(t, live); len(got) != 0 {
		t.Fatalf("drift = %v, want nothing: kubernetes merges these by name, not by position", got)
	}
}

func TestDriftReportsANamedEntryThatIsGone(t *testing.T) {
	live := declaring(
		`{"spec":{"containers":[{"name":"web","image":"web:1"},{"name":"sidecar","image":"log:1"}]}}`,
		map[string]any{"spec": map[string]any{"containers": []any{
			map[string]any{"name": "web", "image": "web:1"},
		}}},
	)

	got := pathsOf(t, live)

	if got["spec.containers[sidecar]"] == "" {
		t.Fatalf("drift = %v, want the missing container named", got)
	}
	if len(got) != 1 {
		t.Fatalf("drift = %v, want only the one that is gone", got)
	}
}

func TestDriftStillComparesAnUnnamedListByPosition(t *testing.T) {
	live := declaring(`{"spec":{"rules":[{"host":"a"},{"host":"b"}]}}`, map[string]any{
		"spec": map[string]any{"rules": []any{
			map[string]any{"host": "a"},
			map[string]any{"host": "c"},
		}},
	})

	got := pathsOf(t, live)

	if got["spec.rules[1].host"] != "b -> c" {
		t.Fatalf("drift = %v, want the second entry compared by position", got)
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

func TestNamedOnlyClaimsAListEveryEntryOfWhichCarriesAName(t *testing.T) {
	cases := []struct {
		name    string
		entries []any
		want    bool
	}{
		{
			name:    "every entry names itself",
			entries: []any{map[string]any{"name": "web"}, map[string]any{"name": "api"}},
			want:    true,
		},
		{
			name:    "one entry does not",
			entries: []any{map[string]any{"name": "web"}, map[string]any{"port": int64(80)}},
			want:    false,
		},
		{
			name:    "the name is not a string",
			entries: []any{map[string]any{"name": int64(1)}},
			want:    false,
		},
		{
			name:    "the entries are not maps",
			entries: []any{"web", "api"},
			want:    false,
		},
		{name: "an empty list", entries: []any{}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := named(tc.entries); got != tc.want {
				t.Fatalf("named(%v) = %v, want %v", tc.entries, got, tc.want)
			}
		})
	}
}

func TestTextRendersEveryValueAJsonDocumentCanHold(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{name: "a string", value: "web", want: "web"},
		{name: "an integer", value: int64(3), want: "3"},
		{name: "a fraction", value: 0.5, want: "0.5"},
		{name: "a boolean", value: true, want: "true"},
		{name: "nothing", value: nil, want: "not set"},
		{name: "a list", value: []any{"a"}, want: `["a"]`},
		{name: "a map", value: map[string]any{"a": "b"}, want: `{"a":"b"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := text(tc.value); got != tc.want {
				t.Fatalf("text(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestANamedListWhoseLiveSideIsNotMapsReportsTheEntryAsGone(t *testing.T) {
	live := declaring(`{"spec":{"containers":[{"name":"web","image":"web:1"}]}}`, map[string]any{
		"spec": map[string]any{"containers": []any{"web"}},
	})

	got := pathsOf(t, live)

	if got["spec.containers[web]"] == "" {
		t.Fatalf("drift = %v, want the named entry reported as missing", got)
	}
}

func TestAnIndexedListWhoseLiveEntryIsNotAMapIsReportedWhole(t *testing.T) {
	live := declaring(`{"spec":{"rules":[{"host":"a"}]}}`, map[string]any{
		"spec": map[string]any{"rules": []any{"a"}},
	})

	got := pathsOf(t, live)

	if got["spec.rules[0]"] != `{"host":"a"} -> a` {
		t.Fatalf("drift = %v, want the whole entry compared", got)
	}
}
