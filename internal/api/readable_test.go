package api

import (
	"slices"
	"testing"
)

func TestNamespacesComeBackFromTheirJSON(t *testing.T) {
	held := ParseNamespaces(`{"spinoza-eks-editor":["payments","storefront"]}`)

	if !slices.Equal(held["spinoza-eks-editor"], []string{"payments", "storefront"}) {
		t.Fatalf("read %v", held)
	}
}

func TestAnythingThatIsNotNamespacesReadsAsNone(t *testing.T) {
	for _, raw := range []string{"", "not json", "[]", "null", "42", `{"ctx":"payments"}`} {
		if got := ParseNamespaces(raw); len(got) != 0 {
			t.Fatalf("%q read as %v", raw, got)
		}
	}
}
