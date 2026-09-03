package resources

import (
	"context"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/rbac"
)

func (m *Manager) Access(ctx context.Context, ref api.ObjectRef) api.Access {
	if m.perms == nil {
		return api.Access{}
	}
	return m.perms.Review(ctx, ref)
}

func (m *Manager) Authorize(ctx context.Context, checks ...access.Check) error {
	if m.perms == nil {
		return nil
	}
	return m.perms.Require(ctx, checks...)
}

func (m *Manager) Reauthorize(ctx context.Context, checks ...access.Check) error {
	if m.perms == nil {
		return nil
	}
	return m.perms.RequireFresh(ctx, checks...)
}

func (m *Manager) AccessEach(ctx context.Context, name string, refs []api.ObjectRef) api.BulkAccess {
	if m.perms == nil {
		return api.BulkAccess{}
	}
	return m.perms.ReviewEach(ctx, name, refs)
}

func (m *Manager) HelmAccess(ctx context.Context, namespace, name string) api.Access {
	if m.perms == nil {
		return api.Access{}
	}
	return m.perms.ReviewRelease(ctx, namespace, m.helm.ReleaseDriver(ctx, namespace, name))
}

func (m *Manager) RBACIndex(ctx context.Context) rbac.Index {
	return rbac.Read(ctx, m, m.descriptors())
}
