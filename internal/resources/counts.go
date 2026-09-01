package resources

import (
	"context"
	"errors"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/podcount"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	countConcurrency = 24
	countTimeout     = 20 * time.Second
	countPerType     = 5 * time.Second
	countUnknown     = -1
)

type CountLimits struct {
	Budget      time.Duration
	PerType     time.Duration
	Concurrency int
}

func (l CountLimits) orDefaults() CountLimits {
	if l.Budget == 0 {
		l.Budget = countTimeout
	}
	if l.PerType == 0 {
		l.PerType = countPerType
	}
	if l.Concurrency == 0 {
		l.Concurrency = countConcurrency
	}
	return l
}

func Count(
	ctx context.Context,
	client metadata.Interface,
	descs []api.ResourceDescriptor,
	limits CountLimits,
) api.ResourceCounts {
	limits = limits.orDefaults()
	bounded, cancel := context.WithTimeout(ctx, limits.Budget)
	defer cancel()

	counts := map[string]int{}
	reasons := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, limits.Concurrency)

	for _, desc := range descs {
		wg.Add(1)
		go safe.Run("counting "+keyOf(desc), func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			total, reason := countOne(bounded, client, desc, limits)
			mu.Lock()
			counts[keyOf(desc)] = total
			if reason != "" {
				reasons[keyOf(desc)] = reason
			}
			mu.Unlock()
		})
	}
	wg.Wait()
	failing, capped := failingPods(bounded, client, descs, limits)
	return api.ResourceCounts{
		Counts:  counts,
		Failing: failing,
		ByPhase: phaseCounted(failing),
		Capped:  capped,
		Errors:  reasons,
	}
}

func phaseCounted(failing map[string]int) []string {
	_, counted := failing[podsKey]
	if !counted {
		return nil
	}
	return []string{podsKey}
}

const podsKey = "/v1/pods"

const unhealthyPods = "status.phase!=Running,status.phase!=Succeeded"

func failingPods(
	ctx context.Context,
	client metadata.Interface,
	descs []api.ResourceDescriptor,
	limits CountLimits,
) (map[string]int, []string) {
	if !counted(descs, podsKey) {
		return nil, nil
	}
	bounded, cancel := context.WithTimeout(ctx, limits.PerType)
	defer cancel()
	got, err := podcount.Count(bounded, client, unhealthyPods)
	if err != nil {
		return nil, nil
	}
	if got.Total == 0 {
		return nil, nil
	}
	if !got.Complete {
		return map[string]int{podsKey: got.Total}, []string{podsKey}
	}
	return map[string]int{podsKey: got.Total}, nil
}

func counted(descs []api.ResourceDescriptor, key string) bool {
	for _, desc := range descs {
		if keyOf(desc) == key {
			return true
		}
	}
	return false
}

func keyOf(desc api.ResourceDescriptor) string {
	return desc.Group + "/" + desc.Version + "/" + desc.Resource
}

func countOne(
	ctx context.Context,
	client metadata.Interface,
	desc api.ResourceDescriptor,
	limits CountLimits,
) (int, string) {
	bounded, cancel := context.WithTimeout(ctx, limits.PerType)
	defer cancel()
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	list, err := client.Resource(gvr).
		Namespace(metav1.NamespaceAll).
		List(bounded, metav1.ListOptions{Limit: 1})
	if err != nil {
		return countUnknown, countReason(ctx, err, limits)
	}
	return len(list.Items) + int(remainingOf(list.GetRemainingItemCount())), ""
}

func countReason(budget context.Context, err error, limits CountLimits) string {
	if budget.Err() != nil {
		return "the " + limits.Budget.String() + " budget for counting ran out before this type was reached"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "counting took longer than " + limits.PerType.String()
	}
	return err.Error()
}

func remainingOf(remaining *int64) int64 {
	if remaining == nil {
		return 0
	}
	if *remaining < 0 {
		return 0
	}
	return *remaining
}
