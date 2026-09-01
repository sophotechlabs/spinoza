package resources

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/prom"
)

type countingProxy struct {
	mu    sync.Mutex
	calls int
	body  string
}

type blockingProxy struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingProxy) Get(ctx context.Context, _ prom.Target, path string, _ map[string]string) ([]byte, error) {
	if path != "api/v1/query" {
		return []byte(`{"status":"success"}`), nil
	}
	select {
	case b.entered <- struct{}{}:
	default:
	}
	select {
	case <-b.release:
		return []byte(labeledFlow), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *countingProxy) Get(ctx context.Context, _ prom.Target, path string, _ map[string]string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path != "api/v1/query" {
		return []byte(`{"status":"success"}`), nil
	}
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return []byte(c.body), nil
}

func (c *countingProxy) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

const labeledFlow = `{"status":"success","data":{"resultType":"vector","result":[
	{"metric":{"source_namespace":"apps","source_workload":"web",
	"destination_namespace":"data","destination_workload":"redis","verdict":"FORWARDED"},
	"value":[1787933018.510,"7"]}
]}}`

func trafficManager(t *testing.T, proxy prom.Proxy, ttl time.Duration) (*Manager, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	client := prom.NewClientWithProxy(
		k8sfake.NewClientset(),
		proxy,
		prom.Target{Namespace: "monitoring", Service: "prometheus", Port: "9090"},
	)
	mgr := NewManager(ctx, Deps{
		Dynamic:    newClient(t),
		Clientset:  k8sfake.NewClientset(),
		Prometheus: client,
		Limits:     Limits{IdleGrace: time.Millisecond, TrafficTTL: ttl},
	})
	return mgr, cancel
}

func TestTrafficSupportSaysItIsNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	support := mgr.TrafficSupport(context.Background())

	if support.Available {
		t.Fatal("traffic was offered without prometheus")
	}
	want := "prometheus is not wired up"
	if support.Reason != want {
		t.Fatalf("reason = %q, want %q", support.Reason, want)
	}
}

func TestTrafficGraphSaysItIsNotWiredUp(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	graph := mgr.TrafficGraph(context.Background())

	want := "prometheus is not wired up"
	if graph.Error != want {
		t.Fatalf("error = %q, want %q", graph.Error, want)
	}
	if len(graph.Nodes) != 0 {
		t.Fatalf("nodes = %d, want none", len(graph.Nodes))
	}
}

func TestTrafficGraphReadsThroughToPrometheus(t *testing.T) {
	proxy := &countingProxy{body: labeledFlow}
	mgr, cancel := trafficManager(t, proxy, time.Minute)
	defer cancel()

	graph := mgr.TrafficGraph(context.Background())

	if graph.Error != "" {
		t.Fatalf("graph reported %q", graph.Error)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("edges = %d, want the one prometheus reported", len(graph.Edges))
	}
	if graph.Edges[0].From != "apps/web" {
		t.Fatalf("edge = %+v", graph.Edges[0])
	}
}

func TestASecondWindowSharesTheFirstTrafficProbe(t *testing.T) {
	proxy := &countingProxy{body: labeledFlow}
	mgr, cancel := trafficManager(t, proxy, time.Minute)
	defer cancel()

	first := mgr.TrafficSupport(context.Background())
	after := proxy.count()
	second := mgr.TrafficSupport(context.Background())

	if !first.Available || !second.Available {
		t.Fatalf("support was refused: %+v and %+v", first, second)
	}
	if proxy.count() != after {
		t.Fatalf("prometheus was asked %d times, want the cached answer reused", proxy.count())
	}
}

func TestTheTrafficProbeIsAskedAgainOnceItGoesStale(t *testing.T) {
	proxy := &countingProxy{body: labeledFlow}
	mgr, cancel := trafficManager(t, proxy, time.Nanosecond)
	defer cancel()

	mgr.TrafficSupport(context.Background())
	after := proxy.count()
	mgr.TrafficSupport(context.Background())

	if proxy.count() <= after {
		t.Fatalf("prometheus was asked %d times, want a stale answer refreshed", proxy.count())
	}
}

func TestATrafficProbeOnACancelledContextSaysWhy(t *testing.T) {
	proxy := &countingProxy{body: labeledFlow}
	mgr, cancel := trafficManager(t, proxy, time.Minute)
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	stop()

	support := mgr.TrafficSupport(ctx)

	if support.Available {
		t.Fatal("a canceled probe was reported as available")
	}
	if !strings.Contains(support.Reason, "context canceled") {
		t.Fatalf("reason = %q, want the cancellation", support.Reason)
	}
}

func TestAWindowCanStopWaitingForAnotherWindowsTrafficProbe(t *testing.T) {
	proxy := &blockingProxy{entered: make(chan struct{}, 1), release: make(chan struct{})}
	mgr, cancel := trafficManager(t, proxy, time.Minute)
	defer cancel()
	firstDone := make(chan struct{})
	var firstAvailable bool
	go func() {
		firstAvailable = mgr.TrafficSupport(context.Background()).Available
		close(firstDone)
	}()
	<-proxy.entered
	waiting, stop := context.WithCancel(context.Background())
	stop()

	second := mgr.TrafficSupport(waiting)

	if second.Available || !strings.Contains(second.Reason, "context canceled") {
		t.Fatalf("support = %+v, want the canceled waiter released", second)
	}
	close(proxy.release)
	<-firstDone
	if !firstAvailable {
		t.Fatal("canceling the waiter canceled the shared probe")
	}
}

func TestTrafficLimitsFallBackToADefaultTTL(t *testing.T) {
	limits := Limits{}.orDefaults()

	if limits.TrafficTTL != defaultTrafficTTL {
		t.Fatalf("TrafficTTL = %v, want %v", limits.TrafficTTL, defaultTrafficTTL)
	}
}
