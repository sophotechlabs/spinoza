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
	mu     sync.Mutex
	filled bool
	who    string
	at     time.Time
	scope  api.Scope
}

type scopeKey struct{}

func WithScopeSlot(ctx context.Context) context.Context {
	return context.WithValue(ctx, scopeKey{}, &scopeSlot{})
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
	defer held.mu.Unlock()
	if held.filled && held.who == asked && s.now().Sub(held.at) <= s.ttl {
		return held.scope
	}
	held.scope = s.readScope(ctx, everyNamespace)
	held.filled = true
	held.who = asked
	held.at = s.now()
	return held.scope
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
	found := unanswered
	for _, one := range decisions {
		if !one.Answered {
			continue
		}
		if one.Allowed {
			return allowed
		}
		found = denied
	}
	return found
}
