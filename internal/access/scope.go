package access

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

const listVerb = "list"

type scopeSlot struct {
	mu      sync.Mutex
	entries map[scopeSlotKey]scopeSlotEntry
}

type scopeSlotKey struct {
	service *Service
	who     string
}

type scopeSlotEntry struct {
	at    time.Time
	scope api.Scope
}

type scopeKey struct{}

func WithScopeSlot(ctx context.Context) context.Context {
	return context.WithValue(ctx, scopeKey{}, &scopeSlot{entries: map[scopeSlotKey]scopeSlotEntry{}})
}

func clusterWide() []Check {
	return []Check{
		{Verb: listVerb, Resource: pods},
		{Verb: listVerb, Group: appsGroup, Resource: "deployments"},
	}
}

func within(namespace string) []Check {
	out := clusterWide()
	for at := range out {
		out[at].Namespace = namespace
	}
	return out
}

func (s *Service) Scope(ctx context.Context, everyNamespace func() []string) api.Scope {
	if s == nil {
		return api.Scope{Everywhere: true}
	}
	held, ok := ctx.Value(scopeKey{}).(*scopeSlot)
	if !ok {
		return s.readScope(ctx, everyNamespace)
	}
	asked := asking(ctx)
	held.mu.Lock()
	key := scopeSlotKey{service: s, who: asked}
	cached, found := held.entries[key]
	if found && s.now().Sub(cached.at) <= s.ttl {
		held.mu.Unlock()
		return cached.scope
	}
	held.mu.Unlock()
	fresh := s.readScope(ctx, everyNamespace)
	finished := s.now()
	held.mu.Lock()
	defer held.mu.Unlock()
	cached, found = held.entries[key]
	if found && finished.Sub(cached.at) <= s.ttl {
		return cached.scope
	}
	held.entries[key] = scopeSlotEntry{at: finished, scope: fresh}
	return fresh
}

func (s *Service) readScope(ctx context.Context, everyNamespace func() []string) api.Scope {
	_, acting := auth.ActingAs(ctx)
	if !acting {
		return api.Scope{Everywhere: true}
	}
	if decide(s.review(ctx, clusterWide())) == allowed {
		return api.Scope{Everywhere: true}
	}
	names := everyNamespace()
	checks := make([]Check, 0, len(names)*2)
	for _, name := range names {
		checks = append(checks, within(name)...)
	}
	decisions := s.review(ctx, checks)
	readable := make([]string, 0, len(names))
	unsure := make([]string, 0, len(names))
	for at, name := range names {
		switch decide(decisions[at*2 : at*2+2]) {
		case allowed:
			readable = append(readable, name)
		case unanswered:
			unsure = append(unsure, name)
		default:
		}
	}
	slices.Sort(readable)
	slices.Sort(unsure)
	return api.Scope{Namespaces: readable, Undecided: unsure}
}

type verdict int

const (
	unanswered verdict = iota
	denied
	allowed
)

func decide(decisions []Decision) verdict {
	if len(decisions) == 0 {
		return unanswered
	}
	found := allowed
	for _, one := range decisions {
		if !one.Answered {
			found = unanswered
			continue
		}
		if !one.Allowed {
			return denied
		}
	}
	return found
}
