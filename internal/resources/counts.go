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

func Count(ctx context.Context, dyn dynamic.Interface, descs []api.ResourceDescriptor) api.ResourceCounts {
	bounded, cancel := context.WithTimeout(ctx, countTimeout)
	defer cancel()

	counts := map[string]int{}
	reasons := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, countConcurrency)

	for _, desc := range descs {
		wg.Add(1)
		go safe.Run("counting "+keyOf(desc), func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			total, reason := countOne(bounded, dyn, desc)
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

func countOne(ctx context.Context, dyn dynamic.Interface, desc api.ResourceDescriptor) (int, string) {
	bounded, cancel := context.WithTimeout(ctx, countPerType)
	defer cancel()
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	list, err := dyn.Resource(gvr).List(bounded, metav1.ListOptions{Limit: 1})
	if err != nil {
		return countUnknown, countReason(ctx, err)
	}
	return len(list.Items) + int(remainingOf(list.GetRemainingItemCount())), ""
}

func countReason(budget context.Context, err error) string {
	if budget.Err() != nil {
		return "the " + countTimeout.String() + " budget for counting ran out before this type was reached"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "counting took longer than " + countPerType.String()
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
