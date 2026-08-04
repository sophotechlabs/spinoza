package jsonschema

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

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

func sourceOf(client openapi.Client) Source {
	return func() openapi.Client {
		return client
	}
}

func clientFor(doc, path string) (*Client, *int) {
	hits := 0
	return NewClient(sourceOf(&fakeClient{
		paths: map[string]openapi.GroupVersion{path: fakeGroupVersion{doc: doc, hits: &hits}},
	})), &hits
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

type blockingClient struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
	paths   map[string]openapi.GroupVersion
}

func newBlockingClient(doc, path string) *blockingClient {
	return &blockingClient{
		entered: make(chan struct{}, 8),
		release: make(chan struct{}),
		paths:   map[string]openapi.GroupVersion{path: fakeGroupVersion{doc: doc}},
	}
}

func (b *blockingClient) Paths() (map[string]openapi.GroupVersion, error) {
	b.calls.Add(1)
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
	return b.paths, nil
}

func TestAStalledFetchDoesNotWedgeTheCacheItself(t *testing.T) {
	blocked := newBlockingClient(podDoc, "api/v1")
	client := NewClient(sourceOf(blocked))
	done := make(chan error, 1)

	go func() {
		_, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
		done <- err
	}()
	<-blocked.entered

	refreshed := make(chan struct{})
	go func() {
		defer close(refreshed)
		client.Refresh()
	}()

	select {
	case <-refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("a stalled openapi fetch held the lock every other caller needs")
	}
	close(blocked.release)
	if err := <-done; err != nil {
		t.Fatalf("for: %v", err)
	}
}

func TestConcurrentCallersShareOneFetch(t *testing.T) {
	blocked := newBlockingClient(podDoc, "api/v1")
	client := NewClient(sourceOf(blocked))
	const callers = 4
	done := make(chan error, callers)

	for range callers {
		go func() {
			_, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
			done <- err
		}()
	}
	<-blocked.entered
	time.Sleep(50 * time.Millisecond)
	close(blocked.release)

	for range callers {
		if err := <-done; err != nil {
			t.Fatalf("for: %v", err)
		}
	}
	if got := blocked.calls.Load(); got != 1 {
		t.Fatalf("openapi fetches = %d, want the callers to share one", got)
	}
}

func TestAWaitingCallerGivesUpWithItsRequest(t *testing.T) {
	blocked := newBlockingClient(podDoc, "api/v1")
	client := NewClient(sourceOf(blocked))
	t.Cleanup(func() { close(blocked.release) })

	go func() {
		_, _ = client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
	}()
	<-blocked.entered

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.For(ctx, GVK{Version: "v1", Kind: "Service"})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the queued caller to leave with its request", err)
	}
}

func TestAFailedFetchIsNotRemembered(t *testing.T) {
	broken := &fakeClient{err: errors.New("apiserver is unreachable")}
	client := NewClient(sourceOf(broken))

	if _, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"}); err == nil {
		t.Fatal("expected the fetch failure to surface")
	}
	broken.err = nil
	broken.paths = map[string]openapi.GroupVersion{"api/v1": fakeGroupVersion{doc: podDoc}}

	if _, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"}); err != nil {
		t.Fatalf("for: %v, want the retry to reach the recovered apiserver", err)
	}
}

func TestForBundlesTheReachableClosure(t *testing.T) {
	client, _ := clientFor(podDoc, "api/v1")

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
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

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
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

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Node"})
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

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Thing"})
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

	raw, err := client.For(context.Background(), GVK{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Kind: "Kustomization"})
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

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
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

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
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

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
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
		if _, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"}); err != nil {
			t.Fatalf("for: %v", err)
		}
	}
	if _, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Service"}); err != nil {
		t.Fatalf("for service: %v", err)
	}

	if *hits != 1 {
		t.Fatalf("document fetched %d times, want 1", *hits)
	}
}

func TestForUnknownKind(t *testing.T) {
	client, _ := clientFor(podDoc, "api/v1")

	_, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Widget"})

	if err == nil {
		t.Fatalf("expected an error for an unknown kind")
	}
}

func TestForUnknownGroupVersion(t *testing.T) {
	client, _ := clientFor(podDoc, "api/v1")

	_, err := client.For(context.Background(), GVK{Group: "apps", Version: "v1", Kind: "Deployment"})

	if err == nil {
		t.Fatalf("expected an error for a group-version the server does not serve")
	}
}

func TestForPathsError(t *testing.T) {
	client := NewClient(sourceOf(&fakeClient{err: errors.New("boom")}))

	_, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})

	if err == nil {
		t.Fatalf("expected the paths error to surface")
	}
}

func TestForSchemaError(t *testing.T) {
	client := NewClient(sourceOf(&fakeClient{
		paths: map[string]openapi.GroupVersion{"api/v1": fakeGroupVersion{err: errors.New("boom")}},
	}))

	_, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})

	if err == nil {
		t.Fatalf("expected the schema error to surface")
	}
}

func TestForMalformedDocument(t *testing.T) {
	client, _ := clientFor("{not json", "api/v1")

	_, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})

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

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
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

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Thing"})
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

	raw, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
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

type swappable struct {
	current openapi.Client
	handles int
}

func (s *swappable) source() openapi.Client {
	s.handles++
	return s.current
}

func docFor(kind string) string {
	return `{"components":{"schemas":{"io.k8s.api.core.v1.` + kind +
		`":{"type":"object","x-kubernetes-group-version-kind":[{"group":"","version":"v1","kind":"` +
		kind + `"}]}}}}`
}

func TestRefreshPicksUpASchemaInstalledLater(t *testing.T) {
	swap := &swappable{current: &fakeClient{
		paths: map[string]openapi.GroupVersion{"api/v1": fakeGroupVersion{doc: docFor("Pod")}},
	}}
	client := NewClient(swap.source)

	_, missing := client.For(context.Background(), GVK{Version: "v1", Kind: "Widget"})
	if missing == nil {
		t.Fatal("expected the unknown kind to fail first")
	}

	swap.current = &fakeClient{
		paths: map[string]openapi.GroupVersion{"api/v1": fakeGroupVersion{doc: docFor("Widget")}},
	}
	client.Refresh()

	_, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Widget"})
	if err != nil {
		t.Fatalf("a schema installed mid-session is still unreachable after a refresh: %v", err)
	}
}

func TestTheHandleIsFetchedEachTimeRatherThanCaptured(t *testing.T) {
	swap := &swappable{current: &fakeClient{
		paths: map[string]openapi.GroupVersion{"api/v1": fakeGroupVersion{doc: docFor("Pod")}},
	}}
	client := NewClient(swap.source)

	_, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}
	client.Refresh()
	_, err = client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}

	if swap.handles < 2 {
		t.Fatalf("asked for the openapi handle %d times; a stale handle never sees a new CRD", swap.handles)
	}
}

func TestRefreshDropsTheMemoisedSchema(t *testing.T) {
	client, hits := clientFor(podDoc, "api/v1")

	_, err := client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}
	before := *hits
	client.Refresh()
	_, err = client.For(context.Background(), GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		t.Fatalf("for: %v", err)
	}

	if *hits == before {
		t.Fatal("the document was served from the old memo after a refresh")
	}
}
