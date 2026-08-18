package resources

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func metricsKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}:  "PodMetricsList",
		{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}: "NodeMetricsList",
		{Group: "", Version: "v1", Resource: "nodes"}:                    "NodeList",
	}
}

func nodeMetric(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "metrics.k8s.io/v1beta1",
		"kind":       "NodeMetrics",
		"metadata":   map[string]any{"name": name},
		"usage":      map[string]any{"cpu": "500m", "memory": "512Mi"},
	}}
}

func seedNodeMetric(t *testing.T, dyn *fake.FakeDynamicClient, name string) {
	t.Helper()
	gvr := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
	_, err := dyn.Resource(gvr).Create(context.Background(), nodeMetric(name), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("seed node metrics: %v", err)
	}
}

func metricsManager(t *testing.T) (*Manager, *fake.FakeDynamicClient, *int64) {
	t.Helper()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), metricsKinds())
	seedNodeMetric(t, dyn, "n1")
	var calls int64
	var mu sync.Mutex
	dyn.PrependReactor("list", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return false, nil, nil
	})
	mgr := NewManager(t.Context(), Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset()})
	return mgr, dyn, &calls
}

func TestMetricsAreReusedWithinTheirWindow(t *testing.T) {
	mgr, _, calls := metricsManager(t)

	first := mgr.Metrics(context.Background())
	before := *calls
	second := mgr.Metrics(context.Background())

	if before == 0 {
		t.Fatal("the first read did not reach the cluster")
	}
	if *calls != before {
		t.Fatalf("list calls = %d, want the cached answer reused", *calls)
	}
	if len(second.Nodes) != len(first.Nodes) {
		t.Fatalf("second read = %+v, want the same as the first", second.Nodes)
	}
}

func TestMetricsAreReadAgainOnceTheWindowPasses(t *testing.T) {
	mgr, _, calls := metricsManager(t)
	mgr.Metrics(context.Background())
	before := *calls

	mgr.now = func() time.Time { return time.Now().Add(2 * defaultMetricsTTL) }
	mgr.Metrics(context.Background())

	if *calls <= before {
		t.Fatalf("list calls = %d, want another read after the window", *calls)
	}
}

func TestConcurrentReadersShareOneMetricsBuild(t *testing.T) {
	mgr, _, calls := metricsManager(t)

	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			mgr.Metrics(context.Background())
		})
	}
	group.Wait()

	if *calls > 3 {
		t.Fatalf("list calls = %d, want one build shared by every reader", *calls)
	}
}

func TestAReaderThatGivesUpWaitingSaysSo(t *testing.T) {
	mgr, dyn, _ := metricsManager(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	dyn.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		once.Do(func() {
			close(entered)
			<-release
		})
		return false, nil, nil
	})
	go func() {
		mgr.Metrics(context.Background())
	}()
	<-entered

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := mgr.Metrics(ctx)
	close(release)

	if got.Error == "" {
		t.Fatal("a reader that gave up came back as a clean read")
	}
}

func TestAFailedMetricsReadIsNotKept(t *testing.T) {
	mgr, dyn, calls := metricsManager(t)
	dyn.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierror("metrics-server is down")
	})

	first := mgr.Metrics(context.Background())
	before := *calls
	mgr.Metrics(context.Background())

	if first.Error == "" {
		t.Fatal("a failed read came back clean")
	}
	if *calls <= before {
		t.Fatalf("list calls = %d, want the failure read again rather than cached", *calls)
	}
}

func TestNodeAllocatableComesFromTheWatchedCache(t *testing.T) {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		metricsKinds(),
		&unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata":   map[string]any{"name": "n1"},
			"status": map[string]any{
				"allocatable": map[string]any{"cpu": "2", "memory": "1024Mi"},
			},
		}},
	)
	seedNodeMetric(t, dyn, "n1")
	descs := map[string]api.ResourceDescriptor{
		"/v1/nodes": {Version: "v1", Resource: "nodes", Kind: "Node"},
	}
	mgr := NewManager(t.Context(), Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Descriptors: descs,
	})

	got := mgr.Metrics(context.Background())

	if got.Nodes["n1"].CPUPercent != 25 {
		t.Fatalf("cpu percent = %d, want a quarter of the watched allocatable", got.Nodes["n1"].CPUPercent)
	}
	mgr.mu.Lock()
	watched := len(mgr.streams)
	mgr.mu.Unlock()
	if watched != 1 {
		t.Fatalf("streams = %d, want the node cache pinned for later reads", watched)
	}
}

func apierror(message string) error {
	return errors.New(message)
}

func countingManager(t *testing.T) (*Manager, *int64) {
	t.Helper()
	scheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(scheme)
	if err != nil {
		t.Fatalf("scheme: %v", err)
	}
	meta := metadatafake.NewSimpleMetadataClient(scheme)
	var calls int64
	var mu sync.Mutex
	meta.PrependReactor("list", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return false, nil, nil
	})
	descs := map[string]api.ResourceDescriptor{
		"apps/v1/deployments": {Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment"},
	}
	mgr := NewManager(t.Context(), Deps{
		Dynamic:     fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), metricsKinds()),
		Metadata:    meta,
		Clientset:   k8sfake.NewClientset(),
		Descriptors: descs,
	})
	return mgr, &calls
}

func TestCountsAreReusedWithinTheirWindow(t *testing.T) {
	mgr, calls := countingManager(t)

	mgr.Counts(context.Background())
	before := *calls
	mgr.Counts(context.Background())

	if before == 0 {
		t.Fatal("the first tally did not reach the cluster")
	}
	if *calls != before {
		t.Fatalf("list calls = %d, want the cached tally reused", *calls)
	}
}

func TestCountsAreTakenAgainOnceTheWindowPasses(t *testing.T) {
	mgr, calls := countingManager(t)
	mgr.Counts(context.Background())
	before := *calls

	mgr.now = func() time.Time { return time.Now().Add(2 * defaultCountsTTL) }
	mgr.Counts(context.Background())

	if *calls <= before {
		t.Fatalf("list calls = %d, want another tally after the window", *calls)
	}
}

func TestWatchedFailuresDoNotLeakIntoTheCachedTally(t *testing.T) {
	cached := api.ResourceCounts{Counts: map[string]int{"/v1/pods": 3}}

	merged := withWatched(cached, map[string]int{"/v1/pods": 2})

	if merged.Failing["/v1/pods"] != 2 {
		t.Fatalf("failing = %+v, want the watched count", merged.Failing)
	}
	if cached.Failing != nil {
		t.Fatalf("the cached tally was written through: %+v", cached.Failing)
	}
}
