package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/prom"
)

func prometheusService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "monitoring",
			Name:      "prometheus-operated",
			Labels:    map[string]string{"operated-prometheus": "true"},
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http-web", Port: 9090}}},
	}
}

func podMetric(namespace, name, cpu, memory string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "metrics.k8s.io/v1beta1",
		"kind":       "PodMetrics",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"containers": []any{map[string]any{
			"name":  "app",
			"usage": map[string]any{"cpu": cpu, "memory": memory},
		}},
	}}
}

func measuredCluster(t *testing.T) *fake.FakeDynamicClient {
	t.Helper()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), metricsKinds())
	seedNodeMetric(t, dyn, "n1")
	gvr := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	_, err := dyn.Resource(gvr).Namespace("prod").
		Create(context.Background(), podMetric("prod", "web", "250m", "512Mi"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("seed pod metrics: %v", err)
	}
	return dyn
}

// A cluster with no metrics database still measures its own pods, and spinoza
// watching those go by is worth more than an error where a chart should be.
func TestMetricHistoryIsMeasuredHereWhenThereIsNoPrometheus(t *testing.T) {
	mgr := NewManager(t.Context(), Deps{Dynamic: measuredCluster(t), Clientset: k8sfake.NewClientset()})

	history, err := mgr.MetricHistory(context.Background(), "prod", "web", time.Hour)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if !history.Sampled {
		t.Fatal("the answer did not say spinoza measured it")
	}
	if len(history.CPU) != 1 {
		t.Fatalf("cpu points = %d, want the one reading taken on the way", len(history.CPU))
	}
	if history.CPU[0].Value != 0.25 {
		t.Fatalf("cpu = %v cores, want the 250m the cluster reported", history.CPU[0].Value)
	}
}

// The panel is often the only page open, so asking it for a chart has to be
// what starts the measuring.
func TestAskingForAChartIsWhatTakesTheFirstReading(t *testing.T) {
	mgr := NewManager(t.Context(), Deps{Dynamic: measuredCluster(t), Clientset: k8sfake.NewClientset()})

	history, err := mgr.MetricHistory(context.Background(), "prod", "web", time.Hour)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history.CPU) == 0 {
		t.Fatal("nothing was measured, so nobody else had asked for metrics first")
	}
}

// refusingProxy is a prometheus that cannot be reached, which is the same
// situation as having none at all.
type refusingProxy struct{}

func (*refusingProxy) Get(context.Context, prom.Target, string, map[string]string) ([]byte, error) {
	return nil, errors.New("dial tcp: connection refused")
}

func TestMetricHistoryFallsBackWhenPrometheusCannotBeReached(t *testing.T) {
	cs := k8sfake.NewClientset()
	mgr := NewManager(t.Context(), Deps{
		Dynamic:    measuredCluster(t),
		Clientset:  cs,
		Prometheus: prom.NewClientWithProxy(cs, &refusingProxy{}, prom.Target{}),
	})

	history, err := mgr.MetricHistory(context.Background(), "prod", "web", time.Hour)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if !history.Sampled {
		t.Fatal("a prometheus that could not be reached left the chart empty")
	}
}

// An answer prometheus gave that could not be read is a different thing from
// prometheus not being there, and is worth saying rather than papering over.
type brokenProxy struct{}

func (*brokenProxy) Get(context.Context, prom.Target, string, map[string]string) ([]byte, error) {
	return []byte(`{"status":"error","errorType":"bad_data","error":"parse error"}`), nil
}

func TestMetricHistoryPassesOnAnErrorThatIsNotAMissingPrometheus(t *testing.T) {
	cs := k8sfake.NewClientset(prometheusService())
	mgr := NewManager(t.Context(), Deps{
		Dynamic:    measuredCluster(t),
		Clientset:  cs,
		Prometheus: prom.NewClientWithProxy(cs, &brokenProxy{}, prom.Target{}),
	})

	_, err := mgr.MetricHistory(context.Background(), "prod", "web", time.Hour)

	if err == nil {
		t.Fatal("an error from prometheus itself was swallowed")
	}
	if errors.Is(err, prom.ErrUnavailable) {
		t.Fatalf("error = %v, want the one prometheus gave rather than a missing one", err)
	}
}
