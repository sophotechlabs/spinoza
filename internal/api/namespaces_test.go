package api

import (
	"slices"
	"testing"
)

func TestTheWorstNamespaceComesFirstOnEverySurface(t *testing.T) {
	rows := []NamespaceCount{
		{Namespace: "quiet", Total: 1},
		{Namespace: "busy", Total: 9},
		{Namespace: "loud", Total: 9, High: 4},
		{Namespace: "alpha", Total: 9},
	}

	slices.SortFunc(rows, WorstNamespaceFirst)

	got := make([]string, 0, len(rows))
	for _, one := range rows {
		got = append(got, one.Namespace)
	}
	want := []string{"loud", "alpha", "busy", "quiet"}
	if !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestTheNamespaceOrderUsesTotalsBeforeNames(t *testing.T) {
	left := NamespaceCount{Namespace: "a", Total: 1}
	right := NamespaceCount{Namespace: "z", Total: 9}
	if WorstNamespaceFirst(left, right) < 0 {
		t.Fatal("a namespace with fewer findings sorted ahead of one with more")
	}
}
