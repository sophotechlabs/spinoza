package server

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/baseline"
)

func TestARejectedBaselineReplacementPreservesTheStoredDocumentAndAllowsRecovery(t *testing.T) {
	const original = `{"takenAt":"2026-09-01T12:00:00Z","checks":["requests-missing"],"counts":{"requests-missing":1},"keys":{"requests-missing\u0000original":"Deployment example/api"}}`
	const replacement = `{"takenAt":"2026-09-23T12:00:00Z","checks":["privileged-containers"],"counts":{},"keys":{}}`
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "malformed", body: `{"takenAt":"2026-09-23","checks":["replacement"`},
		{name: "oversized", body: original + strings.Repeat(" ", maxBaselineBytes)},
		{name: "invalid shape", body: `{"takenAt":"2026-09-23","checks":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts, srv := dashboardPair(t, newPodObject("example", "api"))
			dir := t.TempDir()
			srv.UseBaselines(baseline.Open(dir))
			endpoint := ts.URL + "/api/checks/baseline/file"
			seed := sendBody(t, http.MethodPut, endpoint, []byte(original))
			if seed.StatusCode != http.StatusOK {
				t.Fatalf("initial import status = %d, want 200", seed.StatusCode)
			}
			before := baselineDownload(t, endpoint)
			refused := sendBody(t, http.MethodPut, endpoint, []byte(tc.body))
			if refused.StatusCode != http.StatusBadRequest {
				t.Fatalf("replacement status = %d, want 400", refused.StatusCode)
			}
			srv.UseBaselines(baseline.Open(dir))
			if after := baselineDownload(t, endpoint); after != before {
				t.Fatalf("stored document changed after a refused replacement: %s", after)
			}
			recovered := sendBody(t, http.MethodPut, endpoint, []byte(replacement))
			if recovered.StatusCode != http.StatusOK {
				t.Fatalf("corrected import status = %d, want the import budget released", recovered.StatusCode)
			}
			srv.UseBaselines(baseline.Open(dir))
			after := baselineDownload(t, endpoint)
			if !strings.Contains(after, "2026-09-23T12:00:00Z") || strings.Contains(after, "original") {
				t.Fatalf("corrected document = %s, want only the replacement persisted", after)
			}
		})
	}
}

func baselineDownload(t *testing.T, endpoint string) string {
	t.Helper()
	response := getRaw(t, endpoint)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d, want 200", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("download baseline: %v", err)
	}
	return string(body)
}
