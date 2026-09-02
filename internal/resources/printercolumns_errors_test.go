package resources

import "testing"

func TestAColumnWhoseArrayIndexDoesNotExistIsBlank(t *testing.T) {
	crd := crdWith("v1", column("Tenth condition", "string", ".status.conditions[9].type"))
	shown, ok := layoutOf(crd, "v1")
	if !ok {
		t.Fatal("the valid column definition was not used")
	}

	cells := shown.cells(kustomization())

	if len(cells) != 1 || cells[0] != "" {
		t.Fatalf("cells = %v, want an out-of-range path rendered blank", cells)
	}
}
