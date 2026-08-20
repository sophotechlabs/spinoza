package compare

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func deployment() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":              "web",
			"namespace":         "prod",
			"uid":               "7f1b3f2e-0000-4000-8000-000000000001",
			"resourceVersion":   "918273",
			"generation":        int64(4),
			"creationTimestamp": "2026-07-29T12:43:49Z",
			"selfLink":          "/apis/apps/v1/namespaces/prod/deployments/web",
			"managedFields":     []any{map[string]any{"manager": "kubectl"}},
			"labels":            map[string]any{"app": "web"},
			"annotations": map[string]any{
				lastApplied:                     `{"kind":"Deployment"}`,
				"deployment.kubernetes.io/note": "kept",
			},
			"ownerReferences": []any{map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "ReplicaSet",
				"name":       "web-5c9",
				"uid":        "7f1b3f2e-0000-4000-8000-000000000002",
			}},
		},
		"spec":   map[string]any{"replicas": int64(3)},
		"status": map[string]any{"readyReplicas": int64(3)},
	}}
}

func rendered(t *testing.T, item *unstructured.Unstructured) string {
	t.Helper()
	text, err := YAML(Normalise(item))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return text
}

// what a cluster writes for itself, and which of it survives

func TestTheServersOwnFieldsAreDropped(t *testing.T) {
	text := rendered(t, deployment())

	for _, gone := range []string{
		"uid:",
		"resourceVersion:",
		"generation:",
		"creationTimestamp:",
		"selfLink:",
		"managedFields:",
		"status:",
		lastApplied,
	} {
		if strings.Contains(text, gone) {
			t.Fatalf("%q survived normalisation:\n%s", gone, text)
		}
	}
}

func TestWhatSomebodyAuthoredSurvives(t *testing.T) {
	text := rendered(t, deployment())

	for _, kept := range []string{
		"name: web",
		"namespace: prod",
		"app: web",
		"deployment.kubernetes.io/note: kept",
		"replicas: 3",
		"kind: ReplicaSet",
	} {
		if !strings.Contains(text, kept) {
			t.Fatalf("%q was lost in normalisation:\n%s", kept, text)
		}
	}
}

func TestAnOwnerKeepsEverythingButItsUID(t *testing.T) {
	clean := Normalise(deployment())

	owners := clean.GetOwnerReferences()
	if len(owners) != 1 {
		t.Fatalf("owners = %d, want the one it came with", len(owners))
	}
	if owners[0].UID != "" {
		t.Fatalf("uid = %q, want it dropped", owners[0].UID)
	}
	if owners[0].Name != "web-5c9" || owners[0].Kind != "ReplicaSet" {
		t.Fatalf("owner = %+v, want the rest of it kept", owners[0])
	}
}

func TestAnnotationsGoAwayOnlyWhenNothingIsLeft(t *testing.T) {
	only := deployment()
	unstructured.RemoveNestedField(only.Object, "metadata", "annotations")
	only.SetAnnotations(map[string]string{lastApplied: `{"kind":"Deployment"}`})

	clean := Normalise(only)

	if clean.GetAnnotations() != nil {
		t.Fatalf("annotations = %v, want the key removed with the map", clean.GetAnnotations())
	}
}

func TestAnObjectWithNothingToStripIsUnchanged(t *testing.T) {
	bare := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "settings"},
		"data":       map[string]any{"a": "1"},
	}}

	before, err := YAML(bare)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	after := rendered(t, bare)

	if before != after {
		t.Fatalf("normalisation changed an object that had nothing to strip:\n%s\n%s", before, after)
	}
}

func TestTheSourceIsLeftAlone(t *testing.T) {
	original := deployment()

	Normalise(original)

	if _, found, _ := unstructured.NestedMap(original.Object, "status"); !found {
		t.Fatal("normalisation reached back into the object it was given")
	}
}

// the two sides meet as text

func TestTwoClustersRenderTheSameManifestIdentically(t *testing.T) {
	here := deployment()
	there := deployment()
	there.SetUID("7f1b3f2e-0000-4000-8000-00000000000f")
	there.SetResourceVersion("55")
	unstructured.RemoveNestedField(there.Object, "status")
	if err := unstructured.SetNestedField(there.Object, "different", "status", "phase"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if rendered(t, here) != rendered(t, there) {
		t.Fatalf("the same manifest read differently:\n%s\n%s", rendered(t, here), rendered(t, there))
	}
}

func TestARealDifferenceStillShows(t *testing.T) {
	here := deployment()
	there := deployment()
	if err := unstructured.SetNestedField(there.Object, int64(5), "spec", "replicas"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if rendered(t, here) == rendered(t, there) {
		t.Fatal("a changed replica count was normalised away")
	}
}

func TestKeysComeOutInAStableOrder(t *testing.T) {
	first := rendered(t, deployment())
	second := rendered(t, deployment())

	if first != second {
		t.Fatal("two renders of the same object differ, so a diff would be noise")
	}
}

// reading and writing the text form

func TestRenderedTakesRawYamlThroughTheSamePath(t *testing.T) {
	raw, err := YAML(deployment())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	got, renderErr := Rendered(raw, false)
	if renderErr != nil {
		t.Fatalf("rendered: %v", renderErr)
	}

	if got != rendered(t, deployment()) {
		t.Fatal("yaml in and object in took different paths")
	}
}

func TestRenderedKeepsEverythingWhenAsked(t *testing.T) {
	raw, err := YAML(deployment())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	got, renderErr := Rendered(raw, true)
	if renderErr != nil {
		t.Fatalf("rendered: %v", renderErr)
	}

	if got != raw {
		t.Fatal("the raw form was changed on its way through")
	}
}

func TestSomethingThatIsNotYamlIsRefused(t *testing.T) {
	_, err := Rendered("\tnot: [yaml", false)

	if err == nil {
		t.Fatal("expected unreadable yaml to be refused")
	}
	if !strings.Contains(err.Error(), "could not be read as yaml") {
		t.Fatalf("error = %v, want it to say what failed", err)
	}
}

func TestOrdinaryAnnotationsSurviveTheNormalising(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "settings",
			"namespace": "demo",
			"annotations": map[string]any{
				"meta.helm.sh/release-name":  "podinfo",
				"reloader.stakater.com/auto": "true",
			},
		},
	}}

	clean := Normalise(item)

	annotations := clean.GetAnnotations()
	if len(annotations) != 2 {
		t.Fatalf("annotations = %v, want both kept: only the kubectl blob is noise", annotations)
	}
}

func TestAnObjectWithNoAnnotationsIsLeftAlone(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "settings", "namespace": "demo"},
	}}

	clean := Normalise(item)

	metadata, ok := clean.Object["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %T, want a map", clean.Object["metadata"])
	}
	if _, invented := metadata["annotations"]; invented {
		t.Fatal("normalising invented an annotations map")
	}
}

func TestOwnerReferencesThatAreNotObjectsAreLeftAlone(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":            "web",
			"namespace":       "demo",
			"ownerReferences": []any{"not an owner", map[string]any{"name": "rs", "uid": "abc"}},
		},
	}}

	clean := Normalise(item)

	owners, _, err := unstructured.NestedSlice(clean.Object, "metadata", "ownerReferences")
	if err != nil {
		t.Fatalf("owners: %v", err)
	}
	if len(owners) != 2 {
		t.Fatalf("owners = %v, want both entries kept", owners)
	}
	kept, ok := owners[1].(map[string]any)
	if !ok {
		t.Fatalf("owner = %T, want the map still there", owners[1])
	}
	if _, carries := kept["uid"]; carries {
		t.Fatalf("owner = %v, want the uid dropped", kept)
	}
}
