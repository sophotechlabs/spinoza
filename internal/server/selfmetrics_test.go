package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/telemetry"
)

func TestTheMetricsPageIsPrometheusText(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	rec := httptest.NewRecorder()

	srv.handleSelfMetrics(rec, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))

	if got := rec.Header().Get("Content-Type"); got != telemetry.ContentType() {
		t.Fatalf("content type = %q, want %q", got, telemetry.ContentType())
	}
	if !strings.Contains(rec.Body.String(), "spinoza_") {
		t.Fatalf("the page carried no spinoza metric: %s", rec.Body.String())
	}
}

func TestAMetricsPageNobodyIsListeningForIsNotFatal(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	writer := &deafWriter{err: errors.New("client disconnected")}

	srv.handleSelfMetrics(writer, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))

	if writer.Header().Get("Content-Type") != telemetry.ContentType() {
		t.Fatalf("content type = %q", writer.Header().Get("Content-Type"))
	}
}
