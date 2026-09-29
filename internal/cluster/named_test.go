package cluster

import (
	"slices"
	"testing"
)

func TestEachContextGetsOnlyTheNamespacesNamedForIt(t *testing.T) {
	held := func() map[string][]string {
		return map[string][]string{
			"spinoza-eks-editor": {"payments"},
			"spinoza-eks-viewer": {"storefront"},
		}
	}

	got := namedFor(held, "spinoza-eks-editor")()

	if !slices.Equal(got, []string{"payments"}) {
		t.Fatalf("named = %v, want payments", got)
	}
}

func TestAContextNamedNowhereGetsNothing(t *testing.T) {
	held := func() map[string][]string {
		return map[string][]string{"other": {"payments"}}
	}

	if got := namedFor(held, "spinoza-eks")(); len(got) != 0 {
		t.Fatalf("named = %v, want nothing", got)
	}
}

func TestWithoutASettingSourceNothingIsNamed(t *testing.T) {
	if got := namedFor(nil, "spinoza-eks")(); got != nil {
		t.Fatalf("named = %v, want nothing", got)
	}
}

func TestTheSettingIsReadEachTimeSoAChangeCountsAtOnce(t *testing.T) {
	named := map[string][]string{}
	read := namedFor(func() map[string][]string {
		return named
	}, "spinoza-eks")
	named = map[string][]string{"spinoza-eks": {"payments"}}

	if got := read(); !slices.Equal(got, []string{"payments"}) {
		t.Fatalf("named = %v, want the change without reopening the context", got)
	}
}
