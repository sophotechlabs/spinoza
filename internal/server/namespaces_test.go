package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func namespaceMeta(name string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: schema.GroupVersion{Version: "v1"}.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

func namespacesServer(t *testing.T, objects ...runtime.Object) *httptest.Server {
	t.Helper()
	scheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(scheme)
	if err != nil {
		t.Fatalf("scheme: %v", err)
	}
	meta := metadatafake.NewSimpleMetadataClient(scheme, objects...)
	mgr := resources.NewManager(t.Context(), resources.Deps{Metadata: meta})
	ts := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func TestNamespacesEndpointListsTheCluster(t *testing.T) {
	ts := namespacesServer(t, namespaceMeta("shop"), namespaceMeta("argocd"))

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/namespaces", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got api.Namespaces
	err := json.Unmarshal(body, &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Names) != 2 || got.Names[0] != "argocd" || got.Names[1] != "shop" {
		t.Fatalf("names = %v, want argocd and shop in order", got.Names)
	}
}

func TestNamespacesEndpointIsEmptyWithoutACluster(t *testing.T) {
	ts := namespacesServer(t)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/namespaces", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got api.Namespaces
	err := json.Unmarshal(body, &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Names) != 0 {
		t.Fatalf("names = %v, want none", got.Names)
	}
}
