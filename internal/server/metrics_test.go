package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func TestMetricHistoryRejectsAMissingPod(t *testing.T) {
	ts := debugServer(t, nil)
	res, err := http.Get(ts.URL + "/api/metrics/history?namespace=monitoring")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestMetricHistoryRejectsABadRange(t *testing.T) {
	ts := debugServer(t, nil)
	res, err := http.Get(ts.URL + "/api/metrics/history?namespace=monitoring&pod=loki-0&range=banana")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestMetricHistoryWithoutPrometheusIsMeasuredHere(t *testing.T) {
	ts := debugServer(t, nil)
	res, err := http.Get(ts.URL + "/api/metrics/history?namespace=monitoring&pod=loki-0")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; a missing Prometheus is not the end of the chart", res.StatusCode)
	}
	var history api.MetricHistory
	if decodeErr := json.NewDecoder(res.Body).Decode(&history); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if !history.Sampled {
		t.Fatalf("history = %+v, want it to say spinoza measured this itself", history)
	}
}

type stubPromProxy struct {
	body string
}

func (s *stubPromProxy) Get(_ context.Context, _ prom.Target, path string, _ map[string]string) ([]byte, error) {
	if path == "api/v1/query_range" {
		return []byte(s.body), nil
	}
	return []byte(`{"status":"success"}`), nil
}

func historyServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{podGVR: "PodList"},
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client := prom.NewClientWithProxy(
		k8sfake.NewClientset(),
		&stubPromProxy{body: body},
		prom.Target{Namespace: "monitoring", Service: "prometheus", Port: "9090"},
	)
	mgr := resources.NewManager(ctx, resources.Deps{
		Dynamic:    dyn,
		Clientset:  k8sfake.NewClientset(),
		Prometheus: client,
	})
	ts := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func TestMetricHistoryReturnsTheChartedSeries(t *testing.T) {
	body := `{"status":"success","data":{"result":[{"values":[[1700000000,"0.25"],[1700000060,"0.5"]]}]}}`
	ts := historyServer(t, body)

	res, err := http.Get(ts.URL + "/api/metrics/history?namespace=monitoring&pod=loki-0&range=1h")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var history api.MetricHistory
	decodeErr := json.NewDecoder(res.Body).Decode(&history)
	if decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if history.Namespace != "monitoring" || history.Pod != "loki-0" {
		t.Fatalf("history = %+v, want it to name the pod it was asked about", history)
	}
	if len(history.CPU) != 2 {
		t.Fatalf("cpu points = %d, want 2", len(history.CPU))
	}
	if history.CPU[1].Value != 0.5 {
		t.Fatalf("cpu[1] = %v, want 0.5", history.CPU[1].Value)
	}
	if len(history.Memory) != 2 {
		t.Fatalf("memory points = %d, want 2", len(history.Memory))
	}
}

func TestMetricHistorySurfacesAPrometheusError(t *testing.T) {
	ts := historyServer(t, `{"status":"error","error":"parse error at char 3"}`)

	res, err := http.Get(ts.URL + "/api/metrics/history?namespace=monitoring&pod=loki-0")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode == http.StatusOK {
		t.Fatal("a Prometheus error was reported as a successful, empty chart")
	}
}
