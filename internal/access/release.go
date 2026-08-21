package access

import (
	"context"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

// The helm buttons. A release is not a kubernetes object, so these are asked
// about on their own rather than through capabilitiesFor.
const (
	Install   = "install"
	Upgrade   = "upgrade"
	Rollback  = "rollback"
	Uninstall = "uninstall"
)

// helm keeps every release's history in the namespace it is installed in, as a
// secret or a configmap. Nothing it does to a release works without writing
// that object, which makes it the one thing worth asking about: the chart's own
// resources cannot be known until the chart is rendered.
//
// What each action needs was measured against helm v4 rather than reasoned
// about, one refused verb at a time:
//
//	install    create           a release object for the first revision
//	upgrade    create, update   a new revision, and the old one marked superseded
//	rollback   create, update   the same, going backwards
//	uninstall  delete           the release objects are purged
//
// A dry run writes none of it, which is why previewing is never refused here.
func releaseCapabilities(namespace, driver string) []capability {
	store := Check{Resource: storeFor(driver), Namespace: namespace}
	create := with(store, "create")
	update := with(store, "update")
	return []capability{
		needs(Install, create),
		needs(Upgrade, create, update),
		needs(Rollback, create, update),
		needs(Uninstall, with(store, "delete")),
	}
}

func storeFor(driver string) string {
	if driver == helm.DriverConfigMap {
		return "configmaps"
	}
	return "secrets"
}

// ReviewRelease answers what the cluster would refuse a helm action in this
// namespace, given where the release's history is kept.
func (s *Service) ReviewRelease(ctx context.Context, namespace, driver string) api.Access {
	return s.answer(ctx, releaseCapabilities(namespace, driver))
}
