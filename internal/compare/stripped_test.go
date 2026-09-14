package compare

import (
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func nodePortService(nodePort int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "web", "namespace": "prod", "uid": "u1"},
		"spec": map[string]any{
			"type":      "NodePort",
			"clusterIP": "10.0.0.5",
			"ports":     []any{map[string]any{"port": int64(80), "nodePort": nodePort}},
		},
	}}
}

func webhookWithBundle(caBundle string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "admissionregistration.k8s.io/v1",
		"kind":       "ValidatingWebhookConfiguration",
		"metadata":   map[string]any{"name": "gate", "uid": "u1"},
		"webhooks": []any{map[string]any{
			"name":         "gate.example.com",
			"clientConfig": map[string]any{"caBundle": caBundle, "url": "https://gate"},
		}},
	}}
}

func TestStripNamesTheAllocatedFieldsItLeftOut(t *testing.T) {
	_, stripped := Strip(nodePortService(30080))

	want := []string{"spec.clusterIP", "spec.ports[].nodePort"}
	if !slices.Equal(stripped, want) {
		t.Fatalf("stripped = %q, want %q", stripped, want)
	}
}

func TestStripNamesNothingWhenNothingWasAllocated(t *testing.T) {
	_, stripped := Strip(deployment())

	if len(stripped) != 0 {
		t.Fatalf("stripped = %q, want nothing for a deployment", stripped)
	}
}

func TestTwoServicesThatDifferOnlyInNodePortAreSameWithADifferenceHidden(t *testing.T) {
	found := Kinds([]*unstructured.Unstructured{nodePortService(30080)}, []*unstructured.Unstructured{nodePortService(31000)}, false, false)

	if len(found) != 1 || found[0].Verdict != "same" {
		t.Fatalf("found = %+v, want one pair that reads the same", found)
	}
	if !found[0].HiddenDifferences {
		t.Fatal("the nodePort difference was hidden without saying so")
	}
}

func TestTwoWebhooksThatDifferOnlyInCABundleAreSameWithADifferenceHidden(t *testing.T) {
	found := Kinds([]*unstructured.Unstructured{webhookWithBundle("AAAA")}, []*unstructured.Unstructured{webhookWithBundle("BBBB")}, false, false)

	if len(found) != 1 || found[0].Verdict != "same" || !found[0].HiddenDifferences {
		t.Fatalf("found = %+v, want same with the caBundle difference flagged", found)
	}
}

func TestTwoServicesThatAgreeEvenOnNodePortHideNothing(t *testing.T) {
	found := Kinds([]*unstructured.Unstructured{nodePortService(30080)}, []*unstructured.Unstructured{nodePortService(30080)}, false, false)

	if len(found) != 1 || found[0].Verdict != "same" || found[0].HiddenDifferences {
		t.Fatalf("found = %+v, want same with nothing hidden", found)
	}
}

func TestStatusNoiseDoesNotCountAsAHiddenDifference(t *testing.T) {
	left := deployment()
	right := deployment()
	right.Object["status"] = map[string]any{"readyReplicas": int64(1)}

	found := Kinds([]*unstructured.Unstructured{left}, []*unstructured.Unstructured{right}, false, false)

	if len(found) != 1 || found[0].Verdict != "same" || found[0].HiddenDifferences {
		t.Fatalf("found = %+v, want same with nothing hidden for a status change", found)
	}
}

func TestShowingEverythingComparesWhatTheClusterSent(t *testing.T) {
	found := Kinds([]*unstructured.Unstructured{nodePortService(30080)}, []*unstructured.Unstructured{nodePortService(31000)}, false, true)

	if len(found) != 1 || found[0].Verdict != "differs" || found[0].Lines == 0 {
		t.Fatalf("found = %+v, want the raw objects to differ by their nodePort", found)
	}
}

func TestRenderCarriesWhatItStrippedAndWhatWasAuthored(t *testing.T) {
	raw, err := YAML(nodePortService(30080))
	if err != nil {
		t.Fatal(err)
	}

	rendering, renderErr := Render(raw, false)
	if renderErr != nil {
		t.Fatal(renderErr)
	}

	if !slices.Equal(rendering.Stripped, []string{"spec.clusterIP", "spec.ports[].nodePort"}) {
		t.Fatalf("stripped = %q", rendering.Stripped)
	}
	if rendering.Authored == rendering.Text {
		t.Fatal("the authored view should still carry the allocated fields the text left out")
	}
	kept, keepErr := Render(raw, true)
	if keepErr != nil || kept.Text != raw || len(kept.Stripped) != 0 {
		t.Fatalf("raw rendering = %+v, %v; want the text untouched and nothing stripped", kept, keepErr)
	}
}

func TestUnionMergesAndSortsWithoutRepeats(t *testing.T) {
	got := Union([]string{"spec.ports[].nodePort", "spec.clusterIP"}, []string{"spec.clusterIP", "webhooks[].clientConfig.caBundle"})

	want := []string{"spec.clusterIP", "spec.ports[].nodePort", "webhooks[].clientConfig.caBundle"}
	if !slices.Equal(got, want) {
		t.Fatalf("union = %q, want %q", got, want)
	}
}
