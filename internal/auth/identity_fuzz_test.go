package auth

import (
	"slices"
	"testing"
)

func FuzzRoleAuthorization(f *testing.F) {
	for _, seed := range []struct {
		held   string
		needed string
	}{
		{held: RoleAdmin, needed: RoleViewer},
		{held: RoleViewer, needed: RoleAdmin},
		{held: RoleAdmin, needed: "owner"},
		{held: "owner", needed: "operator"},
		{held: "", needed: ""},
	} {
		f.Add(seed.held, seed.needed)
	}

	f.Fuzz(func(t *testing.T, held, needed string) {
		got := Allows(held, needed)
		if needed == "" {
			if !got {
				t.Fatal("an operation requiring no role was refused")
			}
			return
		}
		neededAt := slices.Index(rolesWeakestFirst, needed)
		if neededAt < 0 {
			if got {
				t.Fatalf("unknown role %q was granted", needed)
			}
			return
		}
		heldAt := slices.Index(rolesWeakestFirst, held)
		if got != (heldAt >= neededAt) {
			t.Fatalf("Allows(%q, %q) = %v", held, needed, got)
		}
	})
}
