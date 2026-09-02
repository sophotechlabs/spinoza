package resources

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	searchConcurrency = 8
	searchTimeout     = 5 * time.Second
	searchPerType     = 2 * time.Second
	searchShortest    = 2
	searchPerKind     = 20
	searchTotal       = 100
)

var searchable = []string{
	"/v1/pods",
	"/v1/services",
	"/v1/configmaps",
	"/v1/secrets",
	"/v1/persistentvolumeclaims",
	"apps/v1/deployments",
	"apps/v1/statefulsets",
	"batch/v1/jobs",
	"batch/v1/cronjobs",
	"networking.k8s.io/v1/ingresses",
}

func searchLimits(limits CountLimits) CountLimits {
	if limits.Budget <= 0 {
		limits.Budget = searchTimeout
	}
	if limits.PerType <= 0 {
		limits.PerType = searchPerType
	}
	if limits.Concurrency <= 0 {
		limits.Concurrency = searchConcurrency
	}
	return limits
}

func searchableTypes(descs []api.ResourceDescriptor) []api.ResourceDescriptor {
	wanted := map[string]bool{}
	for _, key := range searchable {
		wanted[key] = true
	}
	out := make([]api.ResourceDescriptor, 0, len(wanted))
	for _, desc := range descs {
		if wanted[keyOf(desc)] {
			out = append(out, desc)
		}
	}
	return out
}

func Search(
	ctx context.Context,
	meta metadata.Interface,
	descs []api.ResourceDescriptor,
	query string,
	limits CountLimits,
) api.SearchResults {
	needle := strings.ToLower(strings.TrimSpace(query))
	if len([]rune(needle)) < searchShortest {
		return api.SearchResults{}
	}
	limits = searchLimits(limits)
	bounded, cancel := context.WithTimeout(ctx, limits.Budget)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	hits := []api.SearchHit{}
	reasons := map[string]string{}
	truncated := false
	slots := make(chan struct{}, limits.Concurrency)

	for _, desc := range searchableTypes(descs) {
		key := keyOf(desc)
		what := "searching " + key
		wg.Add(1)
		safe.Go(what, func() {
			defer wg.Done()
			defer func() {
				caught := recover()
				if caught == nil {
					return
				}
				safe.Log(what, caught)
				mu.Lock()
				reasons[key] = "spinoza could not finish searching this resource"
				mu.Unlock()
			}()
			slots <- struct{}{}
			defer func() { <-slots }()
			found, cut, err := searchOne(bounded, meta, desc, needle, limits)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				reasons[key] = countReason(ctx, err, limits)
				return
			}
			hits = append(hits, found...)
			if cut {
				truncated = true
			}
		})
	}
	wg.Wait()

	slices.SortFunc(hits, func(left, right api.SearchHit) int {
		if left.Kind != right.Kind {
			return strings.Compare(left.Kind, right.Kind)
		}
		if left.Namespace != right.Namespace {
			return strings.Compare(left.Namespace, right.Namespace)
		}
		return strings.Compare(left.Name, right.Name)
	})
	if len(hits) > searchTotal {
		hits = hits[:searchTotal]
		truncated = true
	}
	return api.SearchResults{Hits: hits, Truncated: truncated, Errors: reasons}
}

func searchOne(
	ctx context.Context,
	meta metadata.Interface,
	desc api.ResourceDescriptor,
	needle string,
	limits CountLimits,
) ([]api.SearchHit, bool, error) {
	bounded, cancel := context.WithTimeout(ctx, limits.PerType)
	defer cancel()
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	list, err := meta.Resource(gvr).Namespace(metav1.NamespaceAll).List(bounded, metav1.ListOptions{})
	if err != nil {
		return nil, false, err
	}
	hits := make([]api.SearchHit, 0, searchPerKind)
	truncated := false
	for _, item := range list.Items {
		if !strings.Contains(strings.ToLower(item.GetName()), needle) {
			continue
		}
		if len(hits) == searchPerKind {
			truncated = true
			break
		}
		hits = append(hits, api.SearchHit{
			Group:     desc.Group,
			Version:   desc.Version,
			Resource:  desc.Resource,
			Kind:      desc.Kind,
			Namespace: item.GetNamespace(),
			Name:      item.GetName(),
		})
	}
	return hits, truncated, nil
}

func (m *Manager) Search(ctx context.Context, query string) api.SearchResults {
	if m.meta == nil {
		return api.SearchResults{}
	}
	descs := m.descriptors()
	flat := make([]api.ResourceDescriptor, 0, len(descs))
	for _, desc := range descs {
		flat = append(flat, desc)
	}
	seen := m.filter(ctx)
	found := Search(auth.AsServer(ctx), m.meta, flat, query, m.limits.Search)
	return m.scopedHits(seen, found)
}

const namespaceResource = "namespaces"

var namespaceGVR = schema.GroupVersionResource{Version: "v1", Resource: namespaceResource}

func (m *Manager) Namespaces(ctx context.Context) api.Namespaces {
	found := m.everyNamespace(ctx)
	if found.Error != "" {
		return found
	}
	seen := m.Scope(ctx)
	if seen.Everywhere {
		return found
	}
	found.Names = seen.Namespaces
	return found
}
