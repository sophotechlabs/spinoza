package resources

import (
	"context"
	"errors"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
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

func Count(ctx context.Context, dyn dynamic.Interface, descs []api.ResourceDescriptor, limits CountLimits) api.ResourceCounts {
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
			total, reason := countOne(bounded, dyn, desc, limits)
			mu.Lock()
			counts[keyOf(desc)] = total
			if reason != "" {
				reasons[keyOf(desc)] = reason
			}
			mu.Unlock()
		})
	}
	wg.Wait()
	return api.ResourceCounts{Counts: counts, Errors: reasons}
}

func keyOf(desc api.ResourceDescriptor) string {
	return desc.Group + "/" + desc.Version + "/" + desc.Resource
}

func countOne(ctx context.Context, dyn dynamic.Interface, desc api.ResourceDescriptor, limits CountLimits) (int, string) {
	bounded, cancel := context.WithTimeout(ctx, limits.PerType)
	defer cancel()
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	list, err := dyn.Resource(gvr).List(bounded, metav1.ListOptions{Limit: 1})
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
