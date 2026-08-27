package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubUpdates struct {
	status api.UpdateStatus
	asked  int
}

func (s *stubUpdates) Status(context.Context) api.UpdateStatus {
	s.asked++
	return s.status
}

// updateServer is a server with nothing but the update endpoint worth asking
// about, so that the checker wired into it is what the test is about.
func updateServer(t *testing.T, checker Updates) *httptest.Server {
	t.Helper()
	srv := New(nil, testAssets(), testToken)
	if checker != nil {
		srv.UseUpdates(checker)
	}
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func readUpdate(t *testing.T, url string) api.UpdateStatus {
	t.Helper()
	res, err := http.Get(url + "/api/update")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var status api.UpdateStatus
	if decodeErr := json.NewDecoder(res.Body).Decode(&status); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	return status
}

func TestTheUpdateEndpointSaysWhatTheCheckerFound(t *testing.T) {
	checker := &stubUpdates{status: api.UpdateStatus{
		Checked:   true,
		Current:   "v1.14.1",
		Latest:    "v1.15.0",
		Available: true,
		Command:   "curl -fsSL https://spinoza.tech/install.sh | sh",
	}}
	ts := updateServer(t, checker)

	status := readUpdate(t, ts.URL)

	if !status.Available || status.Latest != "v1.15.0" {
		t.Fatalf("status = %+v, want what the checker found", status)
	}
	if checker.asked != 1 {
		t.Fatalf("checker asked %d times, want once per request", checker.asked)
	}
}

// A build with no checker at all still answers: not asking is a state the
// browser has to be able to draw, not an error.
func TestTheUpdateEndpointAnswersWithoutAChecker(t *testing.T) {
	ts := updateServer(t, nil)

	status := readUpdate(t, ts.URL)

	if status.Available || status.Checked {
		t.Fatalf("status = %+v, want nothing claimed", status)
	}
	if status.Reason == "" {
		t.Fatal("no reason was given for there being no answer")
	}
}
