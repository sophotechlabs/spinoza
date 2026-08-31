package auth

import (
	"context"
	"testing"
)

func TestARequestWithNoIdentityIsNobody(t *testing.T) {
	_, ok := IdentityFrom(context.Background())
	if ok {
		t.Fatal("a plain context named somebody")
	}
}

func TestAnAnonymousIdentityIsCarriedButNeverActedAs(t *testing.T) {
	ctx := AsServer(t.Context())

	held, known := IdentityFrom(ctx)
	if !known {
		t.Fatal("the anonymous identity was not carried, so a role gate could not read it")
	}
	if held.Role != "" {
		t.Fatalf("role = %q, want it empty", held.Role)
	}
	if _, acting := ActingAs(ctx); acting {
		t.Fatal("spinoza would impersonate nobody, which the apiserver refuses")
	}
}

func TestAnIdentityComesBackTheWayItWentIn(t *testing.T) {
	who := Identity{User: "alice", Groups: []string{"platform"}, Role: RoleEditor, Session: "s1"}

	held, ok := ActingAs(WithIdentity(t.Context(), who))
	if !ok {
		t.Fatal("the identity did not come back")
	}
	if held.User != "alice" || held.Session != "s1" {
		t.Fatalf("identity = %+v, want the one that went in", held)
	}
}

func TestARoleAllowsEverythingWeakerThanItself(t *testing.T) {
	cases := []struct {
		name   string
		held   string
		needed string
		want   bool
	}{
		{name: "nothing is needed", held: "", needed: "", want: true},
		{name: "an admin may edit", held: RoleAdmin, needed: RoleEditor, want: true},
		{name: "an editor may look", held: RoleEditor, needed: RoleViewer, want: true},
		{name: "a viewer may not edit", held: RoleViewer, needed: RoleEditor, want: false},
		{name: "an unknown role may do nothing", held: "nobody", needed: RoleViewer, want: false},
		{name: "no role at all may do nothing", held: "", needed: RoleViewer, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Allows(tc.held, tc.needed); got != tc.want {
				t.Fatalf("Allows(%q, %q) = %v, want %v", tc.held, tc.needed, got, tc.want)
			}
		})
	}
}

func TestOnlyTheThreeNamedRolesAreKnown(t *testing.T) {
	for _, role := range []string{RoleViewer, RoleEditor, RoleAdmin} {
		if !KnownRole(role) {
			t.Fatalf("%q is one of the roles spinoza has, and it was not recognized", role)
		}
	}
	if KnownRole("owner") {
		t.Fatal("a role nobody defined was accepted")
	}
}

func TestCarryMovesTheSignedInUserOntoALongerLivedContext(t *testing.T) {
	who := Identity{User: "alice", Groups: []string{"platform"}}
	request := WithIdentity(t.Context(), who)
	root := context.Background()

	held, ok := ActingAs(Carry(request, root))
	if !ok {
		t.Fatal("the identity did not come across, so anything outliving the request acts as spinoza")
	}
	if held.User != "alice" || held.Groups[0] != "platform" {
		t.Fatalf("identity = %+v, want the one the request carried", held)
	}
	if _, still := ActingAs(Carry(root, root)); still {
		t.Fatal("a request nobody signed in for carried an identity onto the root context")
	}
	if _, anon := ActingAs(Carry(AsServer(t.Context()), root)); anon {
		t.Fatal("spinoza's own work was given somebody to act as")
	}
}
