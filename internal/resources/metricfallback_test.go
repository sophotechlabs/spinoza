package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/auth"
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

func metricsDeniedTo(t *testing.T) *k8sfake.Clientset {
	t.Helper()
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
		if !ok {
			return false, nil, nil
		}
		attributes := review.Spec.ResourceAttributes
		review.Status.Allowed = attributes.Group != "metrics.k8s.io"
		if !review.Status.Allowed {
			review.Status.Reason = "no metrics for you"
		}
		return true, review, nil
	})
	return cs
}

func TestAReaderDeniedMetricsIsNotServedSomebodyElsesSamples(t *testing.T) {
	dyn := measuredCluster(t)
	mgr := NewManager(t.Context(), Deps{Dynamic: dyn, Clientset: metricsDeniedTo(t)})
	primed, err := mgr.MetricHistory(t.Context(), "prod", "web", time.Hour)
	if err != nil || len(primed.CPU) != 1 {
		t.Fatalf("priming as the server: history = %+v, error = %v", primed, err)
	}
	reader := auth.WithIdentity(t.Context(), auth.Identity{User: "reader", Role: auth.RoleViewer})

	after, err := mgr.MetricHistory(reader, "prod", "web", time.Hour)

	if !errors.Is(err, access.ErrDenied) {
		t.Fatalf("error = %v, want the reader's own denial", err)
	}
	if err.Error() != "kubernetes authorization denied: no metrics for you" {
		t.Fatalf("error = %q", err.Error())
	}
	if len(after.CPU) != 0 {
		t.Fatalf("the denied reader still received %d cached samples", len(after.CPU))
	}
}

func TestAReaderAllowedMetricsStillGetsTheSampledHistory(t *testing.T) {
	dyn := measuredCluster(t)
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
		if !ok {
			return false, nil, nil
		}
		review.Status.Allowed = true
		return true, review, nil
	})
	mgr := NewManager(t.Context(), Deps{Dynamic: dyn, Clientset: cs})
	if _, err := mgr.MetricHistory(t.Context(), "prod", "web", time.Hour); err != nil {
		t.Fatalf("priming as the server: %v", err)
	}
	reader := auth.WithIdentity(t.Context(), auth.Identity{User: "reader", Role: auth.RoleViewer})

	history, err := mgr.MetricHistory(reader, "prod", "web", time.Hour)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if !history.Sampled || len(history.CPU) != 1 {
		t.Fatalf("history = %+v, want the sampled series an authorized reader may see", history)
	}
}

func TestTheMetricsGateAsksAboutTheNamespaceThatWasRequested(t *testing.T) {
	var asked []authv1.ResourceAttributes
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
		if !ok {
			return false, nil, nil
		}
		asked = append(asked, *review.Spec.ResourceAttributes)
		review.Status.Allowed = true
		return true, review, nil
	})
	mgr := NewManager(t.Context(), Deps{Dynamic: measuredCluster(t), Clientset: cs})
	reader := auth.WithIdentity(t.Context(), auth.Identity{User: "reader", Role: auth.RoleViewer})

	if _, err := mgr.MetricHistory(reader, "prod", "web", time.Hour); err != nil {
		t.Fatalf("history: %v", err)
	}

	want := authv1.ResourceAttributes{Namespace: "prod", Verb: "list", Group: "metrics.k8s.io", Resource: "pods"}
	found := false
	for _, one := range asked {
		if one == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("questions = %+v, want %+v among them", asked, want)
	}
}

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

func TestMetricHistoryRefusesAnotherNamespace(t *testing.T) {
	mgr := NewManager(t.Context(), Deps{
		Clientset: decidingClientset(false, "not for you"),
	})
	ctx := auth.WithIdentity(t.Context(), auth.Identity{User: "alice"})

	_, err := mgr.MetricHistory(ctx, "prod", "web", time.Hour)

	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("error = %v, want the namespace refused", err)
	}
}
