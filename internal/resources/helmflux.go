package resources

import (
	"context"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

func (m *Manager) fluxOwners(ctx context.Context) map[string]api.ObjectRef {
	found := []helm.FluxRelease{}
	for _, desc := range m.descriptors() {
		if desc.Group != helm.FluxGroup {
			continue
		}
		if desc.Resource != helm.FluxResource {
			continue
		}
		items, err := m.List(ctx, desc)
		if err != nil {
			continue
		}
		for _, item := range items {
			found = append(found, helm.FluxRelease{
				CRNamespace:      item.GetNamespace(),
				CRName:           item.GetName(),
				ReleaseName:      unstr.String(item, "spec", "releaseName"),
				TargetNamespace:  unstr.String(item, "spec", "targetNamespace"),
				StorageNamespace: unstr.String(item, "spec", "storageNamespace"),
				Ref: api.ObjectRef{
					Group:     desc.Group,
					Version:   desc.Version,
					Resource:  desc.Resource,
					Namespace: item.GetNamespace(),
					Name:      item.GetName(),
				},
			})
		}
	}
	return helm.OwnerIndex(found)
}

func ownerRef(owners map[string]api.ObjectRef, namespace, name string) *api.ObjectRef {
	ref, owned := owners[namespace+"/"+name]
	if !owned {
		return nil
	}
	return &ref
}

func decorateOwners(releases []api.HelmRelease, owners map[string]api.ObjectRef) {
	for i := range releases {
		release := &releases[i]
		release.FluxRef = ownerRef(owners, release.Namespace, release.Name)
	}
}
