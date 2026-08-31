package resources

import (
	"context"
	"errors"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

var ErrOutOfScope = errors.New("your account cannot read that namespace")

var ErrClusterWide = errors.New("your account reads named namespaces only, and this kind belongs to no namespace")

type nsFilter struct {
	all   bool
	names map[string]bool
}

func everything() nsFilter {
	return nsFilter{all: true}
}

func filterFor(seen api.Scope) nsFilter {
	if seen.Everywhere {
		return everything()
	}
	names := make(map[string]bool, len(seen.Namespaces))
	for _, one := range seen.Namespaces {
		names[one] = true
	}
	return nsFilter{names: names}
}

func (nf nsFilter) allows(namespace string) bool {
	if nf.all {
		return true
	}
	return nf.names[namespace]
}

func (nf nsFilter) only() string {
	if nf.all {
		return ""
	}
	if len(nf.names) != 1 {
		return ""
	}
	for name := range nf.names {
		return name
	}
	return ""
}

func (m *Manager) everyNamespace(ctx context.Context) api.Namespaces {
	if m.meta == nil {
		return api.Namespaces{}
	}
	bounded, cancel := context.WithTimeout(auth.AsServer(ctx), m.limits.Search.PerType)
	defer cancel()
	list, err := m.meta.Resource(namespaceGVR).List(bounded, metav1.ListOptions{})
	if err != nil {
		return api.Namespaces{Error: err.Error()}
	}
	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	slices.Sort(names)
	return api.Namespaces{Names: names}
}

func (m *Manager) Scope(ctx context.Context) api.Scope {
	_, acting := auth.ActingAs(ctx)
	if !acting {
		return api.Scope{Everywhere: true}
	}
	return m.perms.Scope(ctx, func() []string {
		return m.everyNamespace(ctx).Names
	})
}

func (m *Manager) filter(ctx context.Context) nsFilter {
	return filterFor(m.Scope(ctx))
}

func (m *Manager) admits(seen nsFilter, desc api.ResourceDescriptor, namespace string) error {
	if seen.all {
		return nil
	}
	if !desc.Namespaced {
		return fmt.Errorf("%w: %s", ErrClusterWide, desc.Kind)
	}
	if namespace == "" {
		return nil
	}
	if seen.allows(namespace) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrOutOfScope, namespace)
}

func (m *Manager) scopedHits(seen nsFilter, found api.SearchResults) api.SearchResults {
	if seen.all {
		return found
	}
	kept := make([]api.SearchHit, 0, len(found.Hits))
	for _, hit := range found.Hits {
		if !seen.allows(hit.Namespace) {
			continue
		}
		kept = append(kept, hit)
	}
	found.Hits = kept
	return found
}
