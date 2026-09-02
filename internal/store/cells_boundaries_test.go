package store

import "testing"

func TestAnEmptyCellListKeepsItsJSONShape(t *testing.T) {
	encoded := cellsText(nil)

	if encoded != "[]" {
		t.Fatalf("cells = %q, want an empty JSON list", encoded)
	}
	if decoded := cellsOf(encoded); len(decoded) != 0 {
		t.Fatalf("decoded cells = %v, want none", decoded)
	}
}
