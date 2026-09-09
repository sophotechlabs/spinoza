package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type wasteBackend struct {
	notStubbed

	pods       []*unstructured.Unstructured
	usage      api.Metrics
	history    api.MetricHistory
	historyErr error
	podErr     error
	asked      []api.ObjectRef
}

func (wb *wasteBackend) ListKind(_ context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error) {
	wb.asked = append(wb.asked, ref)
	if ref.Resource != podResourceName {
		return nil, nil
	}
	return wb.pods, wb.podErr
}

func (wb *wasteBackend) Metrics(context.Context) api.Metrics {
	return wb.usage
}

func (wb *wasteBackend) MetricHistory(
	_ context.Context,
	namespace, pod string,
	_ time.Duration,
) (api.MetricHistory, error) {
	if wb.historyErr != nil {
		return api.MetricHistory{}, wb.historyErr
	}
	out := wb.history
	out.Namespace = namespace
	out.Pod = pod
	return out, nil
}

func wastePod(namespace, name, cpu, memory string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec": map[string]any{"containers": []any{map[string]any{
			"resources": map[string]any{"requests": map[string]any{"cpu": cpu, "memory": memory}},
		}}},
	}}
}

func wasteAsked(t *testing.T, backend *wasteBackend, query string) *httptest.ResponseRecorder {
	t.Helper()
	backend.notStubbed = notStubbed{t: t}
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
	req := httptest.NewRequest(http.MethodGet, "/api/waste"+query, http.NoBody)
	recorded := httptest.NewRecorder()
	srv.handleWaste(recorded, req)
	return recorded
}

func wasteDecoded(t *testing.T, recorded *httptest.ResponseRecorder) api.WasteReport {
	t.Helper()
	if recorded.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorded.Code, recorded.Body.String())
	}
	var report api.WasteReport
	err := json.NewDecoder(recorded.Body).Decode(&report)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return report
}

func TestWasteAnswersWithWhatIsReservedAgainstWhatWasMeasuredOverAWindow(t *testing.T) {
	backend := &wasteBackend{
		pods:    []*unstructured.Unstructured{wastePod("shop", "web-1", "400m", "512Mi")},
		history: api.MetricHistory{CPU: []api.MetricPoint{{Value: 0.1}}, Source: "monitoring/prometheus:9090 (https)"},
	}

	report := wasteDecoded(t, wasteAsked(t, backend, ""))

	if !report.Measured {
		t.Fatalf("report = %+v, want it measured", report)
	}
	if report.Window != "the last 30 minutes" {
		t.Errorf("window = %q, want the window said in words", report.Window)
	}
	if report.Source != "monitoring/prometheus:9090 (https)" {
		t.Errorf("source = %q, want where the usage came from", report.Source)
	}
	if len(report.Namespaces) != 1 || report.Namespaces[0].CPUReclaimable != 300 {
		t.Errorf("namespaces = %+v, want 300 millicores reclaimable", report.Namespaces)
	}
	if len(report.Workloads) != 1 || report.Workloads[0].Name != "web-1" {
		t.Errorf("workloads = %+v, want the bare pod named as its own workload", report.Workloads)
	}
}

func TestWasteFallsBackToMetricsServerWhenNoHistoryAnswers(t *testing.T) {
	backend := &wasteBackend{
		pods:       []*unstructured.Unstructured{wastePod("shop", "web-1", "400m", "512Mi")},
		historyErr: errors.New("prometheus is unavailable"),
		usage:      api.Metrics{Pods: map[string]api.ResourceUsage{"shop/web-1": {CPUMilli: 380, MemoryMi: 500}}},
	}

	report := wasteDecoded(t, wasteAsked(t, backend, ""))

	if report.Source != "metrics-server" || report.Window != "right now" {
		t.Errorf("source = %q, window = %q, want the live reading", report.Source, report.Window)
	}
	if report.Namespaces[0].CPUUsed != 380 {
		t.Errorf("cpu used = %d, want what metrics-server said", report.Namespaces[0].CPUUsed)
	}
}

func TestWasteSaysNothingMeasuredUsageWhenNoSourceAnswers(t *testing.T) {
	backend := &wasteBackend{
		pods:       []*unstructured.Unstructured{wastePod("shop", "web-1", "400m", "512Mi")},
		historyErr: errors.New("prometheus is unavailable"),
		usage:      api.Metrics{Error: "metrics-server did not answer"},
	}

	report := wasteDecoded(t, wasteAsked(t, backend, ""))

	if report.Measured {
		t.Error("measured = true, want the report to say nothing measured usage")
	}
	if !strings.Contains(report.Reason, "nothing measured usage") {
		t.Errorf("reason = %q, want it to say nothing measured usage", report.Reason)
	}
	if report.Namespaces[0].CPURequested != 400 {
		t.Errorf("namespaces = %+v, want the reserved columns still filled", report.Namespaces)
	}
}

func TestWasteReadsOnlyTheNamespaceTheRequestNames(t *testing.T) {
	backend := &wasteBackend{
		pods:    []*unstructured.Unstructured{wastePod("shop", "web-1", "400m", "512Mi")},
		history: api.MetricHistory{CPU: []api.MetricPoint{{Value: 0.1}}},
	}

	wasteDecoded(t, wasteAsked(t, backend, "?namespace=shop"))

	if len(backend.asked) == 0 {
		t.Fatal("nothing was listed")
	}
	for _, ref := range backend.asked {
		if ref.Namespace != "shop" {
			t.Errorf("listed %s in %q, want the namespace the request names", ref.Resource, ref.Namespace)
		}
	}
}

func TestWasteReportsAFailedPodListRatherThanAnEmptyTable(t *testing.T) {
	backend := &wasteBackend{podErr: errors.New("pods are forbidden here")}

	recorded := wasteAsked(t, backend, "")

	if recorded.Code == http.StatusOK {
		t.Fatalf("status = %d, want the failed list reported", recorded.Code)
	}
	if !strings.Contains(recorded.Body.String(), "pods are forbidden here") {
		t.Errorf("body = %q, want the reason the list failed", recorded.Body.String())
	}
}
