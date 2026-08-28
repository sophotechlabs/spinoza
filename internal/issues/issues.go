package issues

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

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
) api.IssueQueue {
	at := now()
	snap := collect(ctx, lister, descs)
	found := podFindings(snap)
	reported := map[string]bool{}
	for _, item := range found {
		reported[item.subject.uid()] = true
	}
	found = append(found, workloadFindings(snap, at)...)
	found = append(found, gitopsFindings(snap)...)
	found = append(found, autoscalerFindings(snap)...)
	found = append(found, conditionFindings(snap)...)
	found = append(found, stallFindings(ctx, events, snap, reported, at)...)

	queue := fold(found, snap)
	queue.Error = snap.failures.Message()
	return queue
}
