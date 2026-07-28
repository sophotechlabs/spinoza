package jsonschema

import (
	"encoding/json"
	"errors"
	"testing"

	"k8s.io/client-go/openapi"
)

type fakeGroupVersion struct {
	doc  string
	err  error
	hits *int
}

func (f fakeGroupVersion) Schema(string) ([]byte, error) {
	if f.hits != nil {
		*f.hits++
	}
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.doc), nil
}

func (f fakeGroupVersion) ServerRelativeURL() string {
	return ""
}

type fakeClient struct {
	paths map[string]openapi.GroupVersion
	err   error
}

func (f *fakeClient) Paths() (map[string]openapi.GroupVersion, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.paths, nil
}

const podDoc = `{
  "components": {
    "schemas": {
      "io.k8s.api.core.v1.Pod": {
        "type": "object",
        "x-kubernetes-group-version-kind": [{"group": "", "kind": "Pod", "version": "v1"}],
        "properties": {
          "spec": {"$ref": "#/components/schemas/io.k8s.api.core.v1.PodSpec"},
          "kind": {"type": "string"}
        }
      },
      "io.k8s.api.core.v1.PodSpec": {
        "type": "object",
        "properties": {
          "containers": {
            "type": "array",
            "items": {"$ref": "#/components/schemas/io.k8s.api.core.v1.Container"}
          }
        }
      },
      "io.k8s.api.core.v1.Container": {
        "type": "object",
        "properties": {"name": {"type": "string"}}
      },
      "io.k8s.api.core.v1.Service": {
        "type": "object",
        "x-kubernetes-group-version-kind": [{"group": "", "kind": "Service", "version": "v1"}]
      }
    }
  }
}`

func clientFor(doc, path string) (*Client, *int) {
	hits := 0
	return NewClient(&fakeClient{
		paths: map[string]openapi.GroupVersion{path: fakeGroupVersion{doc: doc, hits: &hits}},
	}), &hits
}

func decode(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	return out
}

func definitions(t *testing.T, bundle map[string]any) map[string]any {
	t.Helper()
	defs := asMap(t, bundle["definitions"])
	return defs
}

func TestForBundlesTheReachableClosure(t *testing.T) {
	client, _ := clientFor(podDoc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}

	bundle := decode(t, raw)
	if bundle["$ref"] != "#/definitions/io.k8s.api.core.v1.Pod" {
		t.Fatalf("$ref = %v", bundle["$ref"])
	}
	if bundle["$schema"] != draft {
		t.Fatalf("$schema = %v", bundle["$schema"])
	}
	defs := definitions(t, bundle)
	for _, want := range []string{
		"io.k8s.api.core.v1.Pod",
		"io.k8s.api.core.v1.PodSpec",
		"io.k8s.api.core.v1.Container",
	} {
		if _, ok := defs[want]; !ok {
			t.Fatalf("definition %q missing", want)
		}
	}
	if _, ok := defs["io.k8s.api.core.v1.Service"]; ok {
		t.Fatalf("unreachable Service definition was kept")
	}
}

func TestForRewritesRefsIntoDefinitions(t *testing.T) {
	client, _ := clientFor(podDoc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}

	defs := definitions(t, decode(t, raw))
	pod := asMap(t, defs["io.k8s.api.core.v1.Pod"])
	props := asMap(t, pod["properties"])
	spec := asMap(t, props["spec"])
	if spec["$ref"] != "#/definitions/io.k8s.api.core.v1.PodSpec" {
		t.Fatalf("spec $ref = %v", spec["$ref"])
	}

	podSpec := asMap(t, defs["io.k8s.api.core.v1.PodSpec"])
	containers := asMap(t, asMap(t, podSpec["properties"])["containers"])
	items := asMap(t, containers["items"])
	if items["$ref"] != "#/definitions/io.k8s.api.core.v1.Container" {
		t.Fatalf("items $ref = %v (refs inside arrays not rewritten)", items["$ref"])
	}
}

func TestForTerminatesOnRecursiveSchemas(t *testing.T) {
	doc := `{"components":{"schemas":{
	  "Node": {
	    "x-kubernetes-group-version-kind": [{"group":"","kind":"Node","version":"v1"}],
	    "properties": {"child": {"$ref": "#/components/schemas/Node"}}
	  }}}}`
	client, _ := clientFor(doc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Node"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}

	defs := definitions(t, decode(t, raw))
	if len(defs) != 1 {
		t.Fatalf("definitions = %d, want 1", len(defs))
	}
}

func TestForKeepsUnresolvableRefsOut(t *testing.T) {
	doc := `{"components":{"schemas":{
	  "Thing": {
	    "x-kubernetes-group-version-kind": [{"group":"","kind":"Thing","version":"v1"}],
	    "properties": {"gone": {"$ref": "#/components/schemas/Missing"}}
	  }}}}`
	client, _ := clientFor(doc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Thing"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}

	defs := definitions(t, decode(t, raw))
	if len(defs) != 1 {
		t.Fatalf("definitions = %d, want 1", len(defs))
	}
}

func TestForUsesTheGroupPath(t *testing.T) {
	doc := `{"components":{"schemas":{
	  "io.fluxcd.toolkit.kustomize.v1.Kustomization": {
	    "x-kubernetes-group-version-kind": [{"group":"kustomize.toolkit.fluxcd.io","kind":"Kustomization","version":"v1"}]
	  }}}}`
	client, _ := clientFor(doc, "apis/kustomize.toolkit.fluxcd.io/v1")

	raw, err := client.For(GVK{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Kind: "Kustomization"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}
	if decode(t, raw)["$ref"] != "#/definitions/io.fluxcd.toolkit.kustomize.v1.Kustomization" {
		t.Fatalf("unexpected root ref")
	}
}

func TestForPrefersTheNameMatchingTheKind(t *testing.T) {
	doc := `{"components":{"schemas":{
	  "aaa.SomethingElse": {"x-kubernetes-group-version-kind": [{"group":"","kind":"Pod","version":"v1"}]},
	  "zzz.v1.Pod": {"x-kubernetes-group-version-kind": [{"group":"","kind":"Pod","version":"v1"}]}
	}}}`
	client, _ := clientFor(doc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}
	if decode(t, raw)["$ref"] != "#/definitions/zzz.v1.Pod" {
		t.Fatalf("$ref = %v, want the .Pod suffixed schema", decode(t, raw)["$ref"])
	}
}

func TestForFallsBackToAnyDeclaringSchema(t *testing.T) {
	doc := `{"components":{"schemas":{
	  "some.Wrapper": {"x-kubernetes-group-version-kind": [{"group":"","kind":"Pod","version":"v1"}]}
	}}}`
	client, _ := clientFor(doc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}
	if decode(t, raw)["$ref"] != "#/definitions/some.Wrapper" {
		t.Fatalf("$ref = %v", decode(t, raw)["$ref"])
	}
}

func TestForIgnoresMalformedGVKExtensions(t *testing.T) {
	doc := `{"components":{"schemas":{
	  "Bad":  {"x-kubernetes-group-version-kind": "not-a-list"},
	  "Also": {"x-kubernetes-group-version-kind": ["not-a-map"]},
	  "Mismatch": {"x-kubernetes-group-version-kind": [{"group":"other","kind":"Pod","version":"v1"},{"group":"","kind":"Pod","version":"v2"}]},
	  "Good": {"x-kubernetes-group-version-kind": [{"group":"","kind":"Pod","version":"v1"}]}
	}}}`
	client, _ := clientFor(doc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}
	if decode(t, raw)["$ref"] != "#/definitions/Good" {
		t.Fatalf("$ref = %v", decode(t, raw)["$ref"])
	}
}

func TestForCachesTheDocumentAndBundle(t *testing.T) {
	client, hits := clientFor(podDoc, "api/v1")

	for range 3 {
		if _, err := client.For(GVK{Version: "v1", Kind: "Pod"}); err != nil {
			t.Fatalf("for: %v", err)
		}
	}
	if _, err := client.For(GVK{Version: "v1", Kind: "Service"}); err != nil {
		t.Fatalf("for service: %v", err)
	}

	if *hits != 1 {
		t.Fatalf("document fetched %d times, want 1", *hits)
	}
}

func TestForUnknownKind(t *testing.T) {
	client, _ := clientFor(podDoc, "api/v1")

	_, err := client.For(GVK{Version: "v1", Kind: "Widget"})

	if err == nil {
		t.Fatalf("expected an error for an unknown kind")
	}
}

func TestForUnknownGroupVersion(t *testing.T) {
	client, _ := clientFor(podDoc, "api/v1")

	_, err := client.For(GVK{Group: "apps", Version: "v1", Kind: "Deployment"})

	if err == nil {
		t.Fatalf("expected an error for a group-version the server does not serve")
	}
}

func TestForPathsError(t *testing.T) {
	client := NewClient(&fakeClient{err: errors.New("boom")})

	_, err := client.For(GVK{Version: "v1", Kind: "Pod"})

	if err == nil {
		t.Fatalf("expected the paths error to surface")
	}
}

func TestForSchemaError(t *testing.T) {
	client := NewClient(&fakeClient{
		paths: map[string]openapi.GroupVersion{"api/v1": fakeGroupVersion{err: errors.New("boom")}},
	})

	_, err := client.For(GVK{Version: "v1", Kind: "Pod"})

	if err == nil {
		t.Fatalf("expected the schema error to surface")
	}
}

func TestForMalformedDocument(t *testing.T) {
	client, _ := clientFor("{not json", "api/v1")

	_, err := client.For(GVK{Version: "v1", Kind: "Pod"})

	if err == nil {
		t.Fatalf("expected a parse error")
	}
}

func TestGVKString(t *testing.T) {
	if got := (GVK{Version: "v1", Kind: "Pod"}).String(); got != "v1/Pod" {
		t.Fatalf("core GVK = %q", got)
	}
	if got := (GVK{Group: "apps", Version: "v1", Kind: "Deployment"}).String(); got != "apps/v1/Deployment" {
		t.Fatalf("grouped GVK = %q", got)
	}
}

func TestForIgnoresNonStringGVKFields(t *testing.T) {
	doc := `{"components":{"schemas":{
	  "Numeric": {"x-kubernetes-group-version-kind": [{"group": 1, "kind": "Pod", "version": "v1"}]},
	  "Good": {"x-kubernetes-group-version-kind": [{"group":"","kind":"Pod","version":"v1"}]}
	}}}`
	client, _ := clientFor(doc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}
	if decode(t, raw)["$ref"] != "#/definitions/Good" {
		t.Fatalf("$ref = %v", decode(t, raw)["$ref"])
	}
}

func TestForLeavesForeignRefsAlone(t *testing.T) {
	doc := `{"components":{"schemas":{
	  "Thing": {
	    "x-kubernetes-group-version-kind": [{"group":"","kind":"Thing","version":"v1"}],
	    "properties": {
	      "numeric": {"$ref": 5},
	      "external": {"$ref": "https://example.test/schema.json"}
	    }
	  }}}}`
	client, _ := clientFor(doc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Thing"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}

	defs := definitions(t, decode(t, raw))
	props := asMap(t, asMap(t, defs["Thing"])["properties"])
	if asMap(t, props["numeric"])["$ref"] != float64(5) {
		t.Fatalf("non-string $ref was altered: %v", props["numeric"])
	}
	if asMap(t, props["external"])["$ref"] != "https://example.test/schema.json" {
		t.Fatalf("external $ref was rewritten: %v", props["external"])
	}
}

func TestForIgnoresNonStringVersionAndKind(t *testing.T) {
	doc := `{"components":{"schemas":{
	  "BadVersion": {"x-kubernetes-group-version-kind": [{"group":"","kind":"Pod","version": 1}]},
	  "BadKind":    {"x-kubernetes-group-version-kind": [{"group":"","kind": 2,"version":"v1"}]},
	  "Good":       {"x-kubernetes-group-version-kind": [{"group":"","kind":"Pod","version":"v1"}]}
	}}}`
	client, _ := clientFor(doc, "api/v1")

	raw, err := client.For(GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}
	if decode(t, raw)["$ref"] != "#/definitions/Good" {
		t.Fatalf("$ref = %v", decode(t, raw)["$ref"])
	}
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want a map", v)
	}
	return m
}
