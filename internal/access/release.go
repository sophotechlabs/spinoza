package access

import (
	"context"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

const (
	Install   = "install"
	Upgrade   = "upgrade"
	Rollback  = "rollback"
	Uninstall = "uninstall"
)

// Helm keeps release history in the namespace as a secret or configmap, and
// nothing works without writing it. Measured against helm v4:
//
//	install    create
//	upgrade    create, update
//	rollback   create, update
//	uninstall  delete
//
// A dry run writes none of it.
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

func (s *Service) ReviewRelease(ctx context.Context, namespace, driver string) api.Access {
	return s.answer(ctx, releaseCapabilities(namespace, driver))
}
