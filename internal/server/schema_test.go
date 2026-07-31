package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/openapi"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const openapiDoc = `{"components":{"schemas":{
  "io.k8s.api.core.v1.Pod": {
    "type": "object",
    "x-kubernetes-group-version-kind": [{"group":"","kind":"Pod","version":"v1"}],
    "properties": {"spec": {"$ref": "#/components/schemas/io.k8s.api.core.v1.PodSpec"}}
  },
  "io.k8s.api.core.v1.PodSpec": {"type": "object"}
}}}`

type stubGroupVersion struct {
	doc string
}

func (s stubGroupVersion) Schema(string) ([]byte, error) {
	return []byte(s.doc), nil
}

func (s stubGroupVersion) ServerRelativeURL() string {
	return ""
}

type stubOpenAPI struct {
	paths map[string]openapi.GroupVersion
}

func (s *stubOpenAPI) Paths() (map[string]openapi.GroupVersion, error) {
	return s.paths, nil
}

func schemaServer(t *testing.T, schemas *jsonschema.Client) *httptest.Server {
	t.Helper()
	listKinds := map[schema.GroupVersionResource]string{podGVR: "PodList"}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("", "v1", "pods"): podDesc(),
	}
	mgr := resources.NewManager(ctx, dyn, k8sfake.NewClientset(), schemas, nil, nil, nil, nil, nil, descs)
	ts := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func stubSchemas() *jsonschema.Client {
	return jsonschema.NewClient(&stubOpenAPI{
		paths: map[string]openapi.GroupVersion{"api/v1": stubGroupVersion{doc: openapiDoc}},
	})
}

func TestSchemaEndpointReturnsABundle(t *testing.T) {
	ts := schemaServer(t, stubSchemas())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/schema?version=v1&kind=Pod", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", resp.Header.Get("Content-Type"))
	}
	bundle := map[string]any{}
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if bundle["$ref"] != "#/definitions/io.k8s.api.core.v1.Pod" {
		t.Fatalf("$ref = %v", bundle["$ref"])
	}
	defs, ok := bundle["definitions"].(map[string]any)
	if !ok {
		t.Fatalf("no definitions in %v", bundle)
	}
	if len(defs) != 2 {
		t.Fatalf("definitions = %d, want 2", len(defs))
	}
}

func TestSchemaEndpointRequiresParams(t *testing.T) {
	ts := schemaServer(t, stubSchemas())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/schema?version=v1", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestSchemaEndpointUnknownKind(t *testing.T) {
	ts := schemaServer(t, stubSchemas())

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/schema?version=v1&kind=Widget", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSchemaEndpointWithoutASchemaSource(t *testing.T) {
	ts := schemaServer(t, nil)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/schema?version=v1&kind=Pod", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if !json.Valid(body) {
		t.Fatalf("body is not json: %s", body)
	}
}
