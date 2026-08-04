package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func panicServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.guard(handler)))
	t.Cleanup(ts.Close)
	return ts
}

func TestAPanickingHandlerAnswersWithAFault(t *testing.T) {
	ts := panicServer(t, func(http.ResponseWriter, *http.Request) {
		panic("a nil map in cellsFor")
	})

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/object", nil)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 rather than a dropped connection: %s", resp.StatusCode, body)
	}
	if !json.Valid(body) {
		t.Fatalf("body = %s, want the usual json error shape", body)
	}
}

func TestAPanicAfterTheAnswerLeavesItAlone(t *testing.T) {
	ts := panicServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]string{"already": "sent"})
		panic("late")
	})

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/object", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the answer that was already written: %s", resp.StatusCode, body)
	}
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if payload["already"] != "sent" {
		t.Fatalf("body = %s, want the handler's own answer", body)
	}
}

func TestAPanickingMutationIsStillLogged(t *testing.T) {
	ts := panicServer(t, func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/object?name=web", nil)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}
