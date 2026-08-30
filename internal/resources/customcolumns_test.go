package resources

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func labelledPod() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "web-a",
			"namespace": "shop",
			"labels": map[string]any{
				"app":                    "web",
				"app.kubernetes.io/name": "storefront",
				"app.kubernetes.io/tier": "front",
			},
			"annotations": map[string]any{
				"deployment.kubernetes.io/revision": "4",
			},
		},
		"spec":   map[string]any{"nodeName": "worker-1"},
		"status": map[string]any{"phase": "Running"},
	}}
}

func withColumns(held map[string][]api.CustomColumn) *Manager {
	return &Manager{columns: func() map[string][]api.CustomColumn { return held }}
}

func podDescriptorFor() api.ResourceDescriptor {
	return api.ResourceDescriptor{Version: "v1", Resource: "pods", Kind: "Pod"}
}

func TestACustomColumnReadsAFieldTheTableDoesNotShow(t *testing.T) {
	mgr := withColumns(map[string][]api.CustomColumn{
		"/v1/pods": {{Name: "Node name", Path: ".spec.nodeName"}},
	})

	shown := withCustom(builtinLayout("Pod"), mgr.customFor(podDescriptorFor()))

	names := columnNames(shown.columns)
	if names[len(names)-1] != "Node name" {
		t.Fatalf("columns = %v, want the custom one last", names)
	}
	cells := shown.cells(labelledPod())
	if cells[len(cells)-1] != "worker-1" {
		t.Fatalf("the custom cell read %q", cells[len(cells)-1])
	}
}

func TestACustomColumnReadsALabelWithDotsInItsKey(t *testing.T) {
	mgr := withColumns(map[string][]api.CustomColumn{
		"/v1/pods": {{Name: "App", Path: `.metadata.labels['app\.kubernetes\.io/name']`}},
	})

	shown := withCustom(builtinLayout("Pod"), mgr.customFor(podDescriptorFor()))

	cells := shown.cells(labelledPod())
	if cells[len(cells)-1] != "storefront" {
		t.Fatalf("the custom cell read %q, want the label value", cells[len(cells)-1])
	}
}

func TestACustomColumnReadsAnAnnotation(t *testing.T) {
	mgr := withColumns(map[string][]api.CustomColumn{
		"/v1/pods": {{Name: "Revision", Path: `.metadata.annotations['deployment\.kubernetes\.io/revision']`}},
	})

	shown := withCustom(builtinLayout("Pod"), mgr.customFor(podDescriptorFor()))

	cells := shown.cells(labelledPod())
	if cells[len(cells)-1] != "4" {
		t.Fatalf("the custom cell read %q", cells[len(cells)-1])
	}
}

func TestAPathWrittenTheWayKubectlTakesItWorksToo(t *testing.T) {
	mgr := withColumns(map[string][]api.CustomColumn{
		"/v1/pods": {{Name: "Node name", Path: "{.spec.nodeName}"}},
	})

	shown := withCustom(builtinLayout("Pod"), mgr.customFor(podDescriptorFor()))

	cells := shown.cells(labelledPod())
	if cells[len(cells)-1] != "worker-1" {
		t.Fatalf("a braced path read %q", cells[len(cells)-1])
	}
}

func TestACustomColumnPointingAtNothingIsEmptyRatherThanMissing(t *testing.T) {
	mgr := withColumns(map[string][]api.CustomColumn{
		"/v1/pods": {{Name: "Nothing", Path: ".spec.notAField"}},
	})

	shown := withCustom(builtinLayout("Pod"), mgr.customFor(podDescriptorFor()))

	cells := shown.cells(labelledPod())
	if len(cells) != len(shown.columns) {
		t.Fatalf("%d cells for %d columns", len(cells), len(shown.columns))
	}
	if cells[len(cells)-1] != "" {
		t.Fatalf("a path pointing at nothing read %q", cells[len(cells)-1])
	}
}

func TestAColumnWithNoNameOrNoPathIsIgnored(t *testing.T) {
	mgr := withColumns(map[string][]api.CustomColumn{
		"/v1/pods": {
			{Name: "", Path: ".spec.nodeName"},
			{Name: "Node name", Path: "   "},
			{Name: "Broken", Path: ".spec[["},
			{Name: "Node name", Path: ".spec.nodeName"},
		},
	})

	custom := mgr.customFor(podDescriptorFor())

	if len(custom) != 1 || custom[0].name != "Node name" {
		t.Fatalf("kept %d columns, want only the one that reads", len(custom))
	}
}

func TestOnlySoManyCustomColumnsAreTaken(t *testing.T) {
	wanted := make([]api.CustomColumn, 0, maxCustomColumns+4)
	for at := range maxCustomColumns + 4 {
		wanted = append(wanted, api.CustomColumn{Name: "c" + string(rune('a'+at)), Path: ".spec.nodeName"})
	}
	mgr := withColumns(map[string][]api.CustomColumn{"/v1/pods": wanted})

	if got := len(mgr.customFor(podDescriptorFor())); got != maxCustomColumns {
		t.Fatalf("kept %d columns, want the cap of %d", got, maxCustomColumns)
	}
}

func TestAKindNobodyAskedAboutKeepsItsOwnColumns(t *testing.T) {
	mgr := withColumns(map[string][]api.CustomColumn{
		"apps/v1/deployments": {{Name: "Node name", Path: ".spec.nodeName"}},
	})

	shown := withCustom(builtinLayout("Pod"), mgr.customFor(podDescriptorFor()))

	if strings.Join(columnNames(shown.columns), ",") !=
		strings.Join(columnNames(builtinLayout("Pod").columns), ",") {
		t.Fatalf("columns = %v, want the built-in ones", columnNames(shown.columns))
	}
}

func TestWithNoColumnsConfiguredNothingChanges(t *testing.T) {
	for _, mgr := range []*Manager{{}, withColumns(nil), withColumns(map[string][]api.CustomColumn{})} {
		if got := mgr.customFor(podDescriptorFor()); len(got) != 0 {
			t.Fatalf("kept %d columns with none configured", len(got))
		}
	}
}

func TestTheColumnsAreReadFromSettingsAsJSON(t *testing.T) {
	held := api.ParseColumns(`{"/v1/pods":[{"name":"Node name","path":".spec.nodeName"}]}`)

	if len(held["/v1/pods"]) != 1 {
		t.Fatalf("read %v", held)
	}
	if held["/v1/pods"][0].Name != "Node name" {
		t.Fatalf("read %+v", held["/v1/pods"][0])
	}
}

func TestSettingsThatAreNotColumnsReadAsNone(t *testing.T) {
	for _, raw := range []string{"", "not json", "[]", "null"} {
		if got := api.ParseColumns(raw); len(got) != 0 {
			t.Fatalf("%q read as %v", raw, got)
		}
	}
}
