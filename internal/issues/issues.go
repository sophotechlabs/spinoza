package issues

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// MaxRows is how many the queue keeps, one cluster or several.
const MaxRows = 500

const (
	defaultBudget       = 20 * time.Second
	defaultStallBudget  = 5 * time.Second
	defaultStallGrace   = 5 * time.Minute
	defaultStuckGrace   = 5 * time.Minute
	defaultReadyGrace   = 2 * time.Minute
	defaultRows         = MaxRows
	defaultChildren     = 50
	defaultCandidates   = 20
	defaultFallback     = 25
	defaultReaders      = 12
	defaultStallReaders = 8
)

type Limits struct {
	Budget      time.Duration
	StallBudget time.Duration
	StallGrace  time.Duration
	ReadyGrace  time.Duration
	Rows        int
	Children    int
	Candidates  int
	Fallback    int
	Readers     int
	StallReader int
}

func (l Limits) orDefaults() Limits {
	if l.Budget == 0 {
		l.Budget = defaultBudget
	}
	if l.StallBudget == 0 {
		l.StallBudget = defaultStallBudget
	}
	if l.StallGrace == 0 {
		l.StallGrace = defaultStallGrace
	}
	if l.ReadyGrace == 0 {
		l.ReadyGrace = defaultReadyGrace
	}
	if l.Rows == 0 {
		l.Rows = defaultRows
	}
	if l.Children == 0 {
		l.Children = defaultChildren
	}
	if l.Candidates == 0 {
		l.Candidates = defaultCandidates
	}
	if l.Fallback == 0 {
		l.Fallback = defaultFallback
	}
	if l.Readers == 0 {
		l.Readers = defaultReaders
	}
	if l.StallReader == 0 {
		l.StallReader = defaultStallReaders
	}
	return l
}

type Lister interface {
	Lease(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error)
	Cached() []api.ResourceDescriptor
}

type Events interface {
	Events(ctx context.Context, namespace, uid string) ([]api.Event, error)
}

func Build(
	ctx context.Context,
	lister Lister,
	events Events,
	descs map[string]api.ResourceDescriptor,
	now func() time.Time,
	limits Limits,
) api.IssueQueue {
	limits = limits.orDefaults()
	at := now()
	bounded, cancel := context.WithTimeout(ctx, limits.Budget)
	defer cancel()
	snap := collect(bounded, lister, descs, limits)
	found := podFindings(snap)
	reported := map[string]bool{}
	for _, item := range found {
		reported[item.subject.uid()] = true
	}
	found = append(found, workloadFindings(snap, at, limits)...)
	found = append(found, gitopsFindings(snap)...)
	found = append(found, autoscalerFindings(snap)...)
	found = append(found, conditionFindings(snap)...)
	found = append(found, clusterFindings(snap, at)...)
	found = append(found, stallFindings(bounded, events, snap, reported, at, limits)...)

	queue := fold(found, snap, limits)
	queue.Error = snap.failures.Message()
	return queue
}
