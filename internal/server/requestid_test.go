package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestEveryAnswerNamesTheRequestItAnswered(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/version", http.NoBody)
	req.Header.Set(AuthHeader, testToken)
	srv.Handler().ServeHTTP(rec, req)

	named := rec.Header().Get(requestHeader)
	if named == "" {
		t.Fatal("the answer carried no request name")
	}
	if len(named) < 8 {
		t.Fatalf("request name %q is too short to be useful", named)
	}
}

func TestAnErrorCarriesTheSameNameAsItsHeader(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/object", http.NoBody)
	req.Header.Set(AuthHeader, testToken)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("status = %d, want a refusal so the body is an error", rec.Code)
	}
	var failure api.Failure
	if err := json.Unmarshal(rec.Body.Bytes(), &failure); err != nil {
		t.Fatalf("the body did not parse: %v (%s)", err, rec.Body.String())
	}
	if failure.Request == "" {
		t.Fatalf("the error named no request: %s", rec.Body.String())
	}
	if failure.Request != rec.Header().Get(requestHeader) {
		t.Fatalf("body says %q, header says %q", failure.Request, rec.Header().Get(requestHeader))
	}
	if failure.Message == "" {
		t.Fatal("the error carried no message")
	}
}

func TestTwoRequestsAreNamedDifferently(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	handler := srv.Handler()
	seen := map[string]bool{}

	for range 20 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/version", http.NoBody)
		req.Header.Set(AuthHeader, testToken)
		handler.ServeHTTP(rec, req)
		seen[rec.Header().Get(requestHeader)] = true
	}

	if len(seen) != 20 {
		t.Fatalf("20 requests were given %d names", len(seen))
	}
	for name := range seen {
		if strings.Contains(name, "=") {
			t.Fatalf("request name %q is padded", name)
		}
	}
}
