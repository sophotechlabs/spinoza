package unstr

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func object(fields map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: fields}
}

func conditioned(entries ...any) *unstructured.Unstructured {
	return object(map[string]any{
		"status": map[string]any{"conditions": entries},
	})
}

func TestStringReadsANestedField(t *testing.T) {
	obj := object(map[string]any{"spec": map[string]any{"url": "https://charts.example.com"}})

	if got := String(obj, "spec", "url"); got != "https://charts.example.com" {
		t.Fatalf("String = %q", got)
	}
}

func TestStringIsEmptyForWhatIsNotThere(t *testing.T) {
	obj := object(map[string]any{"spec": map[string]any{"replicas": int64(3)}})

	cases := map[string][]string{
		"a missing field":  {"spec", "url"},
		"a missing parent": {"status", "url"},
		"the wrong type":   {"spec", "replicas"},
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			if got := String(obj, path...); got != "" {
				t.Fatalf("String = %q, want empty", got)
			}
		})
	}
}

func TestBoolReadsANestedFlag(t *testing.T) {
	obj := object(map[string]any{"spec": map[string]any{"suspend": true}})

	if !Bool(obj, "spec", "suspend") {
		t.Fatal("Bool did not read the flag")
	}
	if Bool(obj, "spec", "missing") {
		t.Fatal("Bool invented a flag")
	}
}

func TestIntReadsANestedNumber(t *testing.T) {
	obj := object(map[string]any{"status": map[string]any{"readyReplicas": int64(4)}})

	if got := Int(obj, "status", "readyReplicas"); got != 4 {
		t.Fatalf("Int = %d", got)
	}
	if got := Int(obj, "status", "missing"); got != 0 {
		t.Fatalf("Int = %d, want zero for what is not there", got)
	}
}

func TestSliceReadsANestedList(t *testing.T) {
	obj := object(map[string]any{"spec": map[string]any{"containers": []any{"app", "sidecar"}}})

	if got := Slice(obj, "spec", "containers"); len(got) != 2 {
		t.Fatalf("Slice = %v", got)
	}
	if got := Slice(obj, "spec", "missing"); got != nil {
		t.Fatalf("Slice = %v, want nil for what is not there", got)
	}
}

func TestMapReadsANestedObject(t *testing.T) {
	obj := object(map[string]any{"status": map[string]any{"usage": map[string]any{"cpu": "5m"}}})

	usage, ok := Map(obj, "status", "usage")
	if !ok {
		t.Fatal("Map did not read the object")
	}
	if usage["cpu"] != "5m" {
		t.Fatalf("usage = %v", usage)
	}
	if _, ok := Map(obj, "status", "missing"); ok {
		t.Fatal("Map invented an object")
	}
}

func TestAtReadsAStringKey(t *testing.T) {
	entry := map[string]any{"type": "Ready", "observedGeneration": int64(2)}

	if got := At(entry, "type"); got != "Ready" {
		t.Fatalf("At = %q", got)
	}
	if got := At(entry, "observedGeneration"); got != "" {
		t.Fatalf("At = %q, want empty for a value that is not a string", got)
	}
	if got := At(entry, "missing"); got != "" {
		t.Fatalf("At = %q, want empty for a key that is not there", got)
	}
}

func TestReadyReportsTheStatusAndMessage(t *testing.T) {
	obj := conditioned(
		map[string]any{"type": "Reconciling", "status": "True"},
		map[string]any{"type": "Ready", "status": "False", "message": "artifact not found"},
	)

	status, message := Ready(obj)

	if status != "False" {
		t.Fatalf("status = %q", status)
	}
	if message != "artifact not found" {
		t.Fatalf("message = %q", message)
	}
}

func TestReadyIsEmptyWithoutAReadyCondition(t *testing.T) {
	cases := map[string]*unstructured.Unstructured{
		"no conditions at all":              object(map[string]any{"status": map[string]any{}}),
		"no ready condition":                conditioned(map[string]any{"type": "Reconciling", "status": "True"}),
		"a condition that is not an object": conditioned("Ready"),
	}
	for name, obj := range cases {
		t.Run(name, func(t *testing.T) {
			status, message := Ready(obj)
			if status != "" || message != "" {
				t.Fatalf("Ready = %q, %q, want both empty", status, message)
			}
		})
	}
}

func TestReadySummaryNamesWhatIsWrong(t *testing.T) {
	cases := []struct {
		name string
		obj  *unstructured.Unstructured
		want string
	}{
		{
			name: "ready",
			obj:  conditioned(map[string]any{"type": "Ready", "status": "True"}),
			want: "Ready",
		},
		{
			name: "not ready with a reason",
			obj:  conditioned(map[string]any{"type": "Ready", "status": "False", "reason": "ArtifactFailed"}),
			want: "ArtifactFailed",
		},
		{
			name: "not ready without a reason",
			obj:  conditioned(map[string]any{"type": "Ready", "status": "False"}),
			want: "NotReady",
		},
		{
			name: "no ready condition",
			obj:  conditioned(map[string]any{"type": "Reconciling", "status": "True"}),
			want: "",
		},
		{
			name: "a condition that is not an object",
			obj:  conditioned("Ready"),
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReadySummary(tc.obj); got != tc.want {
				t.Fatalf("ReadySummary = %q, want %q", got, tc.want)
			}
		})
	}
}
