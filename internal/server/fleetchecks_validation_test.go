package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFleetFindingPageRequiresACheckBeforeReadingTheFleet(t *testing.T) {
	srv := New(&stubBackendCluster{}, testAssets(), "")
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/checks/findings/fleet", http.NoBody)

	srv.fleetCheckPage(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "check is required") {
		t.Fatalf("body = %s, want the missing check named", recorder.Body.String())
	}
}
