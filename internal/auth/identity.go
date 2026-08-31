package auth

import (
	"context"
	"slices"
)

const (
	RoleViewer = "viewer"
	RoleEditor = "editor"
	RoleAdmin  = "admin"
)

var rolesWeakestFirst = []string{RoleViewer, RoleEditor, RoleAdmin}

type Identity struct {
	User    string
	Groups  []string
	Role    string
	Session string
}

func (who Identity) Anonymous() bool {
	return who.User == ""
}

type identityKey struct{}

func WithIdentity(ctx context.Context, who Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, who)
}

func IdentityFrom(ctx context.Context) (Identity, bool) {
	who, ok := ctx.Value(identityKey{}).(Identity)
	if !ok {
		return Identity{}, false
	}
	return who, true
}

func Carry(from, onto context.Context) context.Context {
	who, ok := ActingAs(from)
	if !ok {
		return onto
	}
	return WithIdentity(onto, who)
}

func AsServer(ctx context.Context) context.Context {
	return WithIdentity(ctx, Identity{})
}

func ActingAs(ctx context.Context) (Identity, bool) {
	who, ok := IdentityFrom(ctx)
	if !ok {
		return Identity{}, false
	}
	if who.Anonymous() {
		return Identity{}, false
	}
	return who, true
}

func KnownRole(role string) bool {
	return slices.Contains(rolesWeakestFirst, role)
}

func Allows(held, needed string) bool {
	if needed == "" {
		return true
	}
	return slices.Index(rolesWeakestFirst, held) >= slices.Index(rolesWeakestFirst, needed)
}
