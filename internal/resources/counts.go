package resources

import (
	"context"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	countConcurrency = 24
	countTimeout     = 20 * time.Second
	countPerType     = 5 * time.Second
	countUnknown     = -1
)

func Count(ctx context.Context, dyn dynamic.Interface, descs []api.ResourceDescriptor) map[string]int {
	bounded, cancel := context.WithTimeout(ctx, countTimeout)
	defer cancel()

	counts := map[string]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, countConcurrency)

	for _, desc := range descs {
		wg.Add(1)
		go func(desc api.ResourceDescriptor) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			total := countOne(bounded, dyn, desc)
			mu.Lock()
			counts[keyOf(desc)] = total
			mu.Unlock()
		}(desc)
	}
	wg.Wait()
	return counts
}

func keyOf(desc api.ResourceDescriptor) string {
	return desc.Group + "/" + desc.Version + "/" + desc.Resource
}

func countOne(ctx context.Context, dyn dynamic.Interface, desc api.ResourceDescriptor) int {
	bounded, cancel := context.WithTimeout(ctx, countPerType)
	defer cancel()
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	list, err := dyn.Resource(gvr).List(bounded, metav1.ListOptions{Limit: 1})
	if err != nil {
		return countUnknown
	}
	return len(list.Items) + int(remainingOf(list.GetRemainingItemCount()))
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
