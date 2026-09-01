package compare

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestMutatingWebhookCABundlesAreDropped(t *testing.T) {
	here := webhook("first-cluster")
	here.SetKind(mutatingKind)
	there := webhook("second-cluster")
	there.SetKind(mutatingKind)

	left := rendered(t, Normalise(here))
	right := rendered(t, Normalise(there))

	if left != right {
		t.Fatalf("the same mutating webhook differed across clusters:\n%s\n---\n%s", left, right)
	}
	if strings.Contains(left, "caBundle") {
		t.Fatalf("the injected CA bundle survived:\n%s", left)
	}
}

func TestMalformedServicePortEntriesSurviveNormalisation(t *testing.T) {
	item := service("10.43.0.5", 31080)
	ports := []any{
		"not an object",
		map[string]any{"name": "http", "port": int64(80), "nodePort": int64(31080)},
	}
	if err := unstructured.SetNestedSlice(item.Object, ports, specField, "ports"); err != nil {
		t.Fatalf("seed ports: %v", err)
	}

	clean := Normalise(item)
	found, ok, err := unstructured.NestedSlice(clean.Object, specField, "ports")
	if err != nil || !ok {
		t.Fatalf("ports = %v found=%t err=%v", found, ok, err)
	}
	if found[0] != "not an object" {
		t.Fatalf("malformed entry = %#v, want it preserved", found[0])
	}
	port, ok := found[1].(map[string]any)
	if !ok {
		t.Fatalf("valid entry = %T, want a map", found[1])
	}
	if _, carried := port["nodePort"]; carried {
		t.Fatalf("server allocation survived: %#v", port)
	}
	if port["port"] != int64(80) {
		t.Fatalf("authored port was changed: %#v", port)
	}
}

func TestMalformedWebhookEntriesSurviveNormalisation(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "admissionregistration.k8s.io/v1",
		"kind":       validatingKind,
		"metadata":   map[string]any{"name": "guard"},
		"webhooks": []any{
			"not an object",
			map[string]any{"name": "missing.example.com", "clientConfig": "not an object"},
			map[string]any{
				"name": "valid.example.com",
				"clientConfig": map[string]any{
					"caBundle": "injected",
					"service":  map[string]any{"name": "guard", "namespace": "prod"},
				},
			},
		},
	}}

	clean := Normalise(item)
	hooks, found, err := unstructured.NestedSlice(clean.Object, "webhooks")
	if err != nil || !found {
		t.Fatalf("webhooks = %v found=%t err=%v", hooks, found, err)
	}
	if len(hooks) != 3 {
		t.Fatalf("webhooks = %d, want every entry preserved", len(hooks))
	}
	if hooks[0] != "not an object" {
		t.Fatalf("malformed hook = %#v", hooks[0])
	}
	missing, ok := hooks[1].(map[string]any)
	if !ok || missing["clientConfig"] != "not an object" {
		t.Fatalf("malformed client config = %#v", hooks[1])
	}
	valid, ok := hooks[2].(map[string]any)
	if !ok {
		t.Fatalf("valid hook = %T", hooks[2])
	}
	config, ok := valid["clientConfig"].(map[string]any)
	if !ok {
		t.Fatalf("valid client config = %T", valid["clientConfig"])
	}
	if _, carried := config["caBundle"]; carried {
		t.Fatalf("CA bundle survived: %#v", config)
	}
	if config["service"] == nil {
		t.Fatalf("authored service was dropped: %#v", config)
	}
}

func TestWrongShapedAllocationsAreLeftAlone(t *testing.T) {
	service := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       serviceKind,
		"metadata":   map[string]any{"name": "web"},
		"spec":       map[string]any{"ports": "not a list", "clusterIPs": "not a list"},
	}}
	webhook := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "admissionregistration.k8s.io/v1",
		"kind":       validatingKind,
		"metadata":   map[string]any{"name": "guard"},
		"webhooks":   "not a list",
	}}

	cleanService := Normalise(service)
	cleanWebhook := Normalise(webhook)

	ports, _, _ := unstructured.NestedFieldNoCopy(cleanService.Object, specField, "ports")
	if ports != "not a list" {
		t.Fatalf("ports = %#v, want the malformed value preserved", ports)
	}
	clusterIPs, _, _ := unstructured.NestedFieldNoCopy(cleanService.Object, specField, "clusterIPs")
	if clusterIPs != "not a list" {
		t.Fatalf("clusterIPs = %#v, want the malformed value preserved", clusterIPs)
	}
	hooks, _, _ := unstructured.NestedFieldNoCopy(cleanWebhook.Object, "webhooks")
	if hooks != "not a list" {
		t.Fatalf("webhooks = %#v, want the malformed value preserved", hooks)
	}
}

func TestYAMLReportsAnUnsupportedValue(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{"unsupported": make(chan int)}}

	_, err := YAML(item)

	if err == nil {
		t.Fatal("an unsupported value was rendered")
	}
	if !strings.Contains(err.Error(), "could not be written as yaml") {
		t.Fatalf("error = %v", err)
	}
}
