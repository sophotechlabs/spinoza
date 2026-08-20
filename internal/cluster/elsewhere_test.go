package cluster

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func somewhere() api.ContextRef {
	return api.ContextRef{Name: "p-mk2"}
}

func someObject() api.ObjectRef {
	return api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "prod",
		Name:      "web",
	}
}

func TestReadingAnotherContextGoesThroughTheReader(t *testing.T) {
	cluster := &Cluster{}
	var asked api.ContextRef
	cluster.useReader(func(_ context.Context, ref api.ContextRef, _ api.ObjectRef) (string, error) {
		asked = ref
		return "kind: Deployment\n", nil
	})

	yaml, err := cluster.Read(t.Context(), somewhere(), someObject())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if yaml != "kind: Deployment\n" {
		t.Fatalf("yaml = %q", yaml)
	}
	if asked.Name != "p-mk2" {
		t.Fatalf("read from %q, want the context that was named", asked.Name)
	}
}

func TestAReadOfAnotherContextThatFailsSaysWhy(t *testing.T) {
	cluster := &Cluster{}
	cluster.useReader(func(context.Context, api.ContextRef, api.ObjectRef) (string, error) {
		return "", errors.New("that cluster is unreachable")
	})

	_, err := cluster.Read(t.Context(), somewhere(), someObject())

	if err == nil {
		t.Fatal("a failed read handed back an empty document instead")
	}
}

func TestReadingAnotherContextWithNoReaderWiredUp(t *testing.T) {
	cluster := &Cluster{}

	_, err := cluster.Read(t.Context(), somewhere(), someObject())

	if !errors.Is(err, api.ErrInternal) {
		t.Fatalf("error = %v, want an internal one", err)
	}
}

func TestListingAnotherContextGoesThroughTheLister(t *testing.T) {
	cluster := &Cluster{}
	var asked api.ObjectRef
	cluster.useLister(func(
		_ context.Context,
		_ api.ContextRef,
		target api.ObjectRef,
	) ([]*unstructured.Unstructured, error) {
		asked = target
		return []*unstructured.Unstructured{{}, {}}, nil
	})

	found, err := cluster.List(t.Context(), somewhere(), someObject())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("found %d, want what the lister handed back", len(found))
	}
	if asked.Resource != "deployments" {
		t.Fatalf("listed %q, want the kind that was named", asked.Resource)
	}
}

func TestAListOfAnotherContextThatFailsSaysWhy(t *testing.T) {
	cluster := &Cluster{}
	cluster.useLister(func(
		context.Context,
		api.ContextRef,
		api.ObjectRef,
	) ([]*unstructured.Unstructured, error) {
		return nil, errors.New("that cluster is unreachable")
	})

	_, err := cluster.List(t.Context(), somewhere(), someObject())

	if err == nil {
		t.Fatal("a failed list handed back an empty kind instead")
	}
}

func TestListingAnotherContextWithNoListerWiredUp(t *testing.T) {
	cluster := &Cluster{}

	_, err := cluster.List(t.Context(), somewhere(), someObject())

	if !errors.Is(err, api.ErrInternal) {
		t.Fatalf("error = %v, want an internal one", err)
	}
}
