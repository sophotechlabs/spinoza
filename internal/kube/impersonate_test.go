package kube

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/transport"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

type recordingTripper struct {
	seen *http.Request
}

func (rt *recordingTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.seen = req
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}

func roundTrip(t *testing.T, req *http.Request) (*http.Request, error) {
	t.Helper()
	recorder := &recordingTripper{}
	_, err := Impersonating(recorder).RoundTrip(req)
	return recorder.seen, err
}

func TestAnUnattributedRequestGoesAsSpinozaItself(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", http.NoBody)

	seen, err := roundTrip(t, req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if seen.Header.Get(transport.ImpersonateUserHeader) != "" {
		t.Fatal("a request nobody signed in for still impersonated somebody")
	}
	if seen != req {
		t.Fatal("the request was cloned when nothing needed changing")
	}
}

func TestARequestForSomebodyCarriesTheirNameAndGroups(t *testing.T) {
	ctx := auth.WithIdentity(t.Context(), auth.Identity{
		User:   "alice@example.com",
		Groups: []string{"platform", "sre"},
	})
	req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", http.NoBody).WithContext(ctx)

	seen, err := roundTrip(t, req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if seen.Header.Get(transport.ImpersonateUserHeader) != "alice@example.com" {
		t.Fatalf("user header = %q", seen.Header.Get(transport.ImpersonateUserHeader))
	}
	groups := strings.Join(seen.Header.Values(transport.ImpersonateGroupHeader), ",")
	if groups != "platform,sre" {
		t.Fatalf("group headers = %q, want both", groups)
	}
	if req.Header.Get(transport.ImpersonateUserHeader) != "" {
		t.Fatal("the caller's own request was modified, which a round tripper may not do")
	}
}

func TestGroupHeadersLeftOverFromAnEarlierIdentityAreCleared(t *testing.T) {
	ctx := auth.WithIdentity(t.Context(), auth.Identity{User: "alice", Groups: []string{"platform"}})
	req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", http.NoBody).WithContext(ctx)
	req.Header.Add(transport.ImpersonateGroupHeader, "system:masters")

	seen, err := roundTrip(t, req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	groups := seen.Header.Values(transport.ImpersonateGroupHeader)
	if len(groups) != 1 || groups[0] != "platform" {
		t.Fatalf("group headers = %v, want only the signed-in user's", groups)
	}
}

func TestANameNoHeaderCanCarryStopsTheRequest(t *testing.T) {
	ctx := auth.WithIdentity(t.Context(), auth.Identity{User: "alice\r\nX-Evil: 1"})
	req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", http.NoBody).WithContext(ctx)

	seen, err := roundTrip(t, req)
	if err == nil {
		t.Fatal("a username that could forge a header was sent anyway")
	}
	if seen != nil {
		t.Fatal("the request reached the apiserver")
	}
	if !strings.Contains(err.Error(), "impersonation header cannot") {
		t.Fatalf("error = %q, want it to name the reason", err.Error())
	}
}

func TestAGroupNoHeaderCanCarryIsLeftOutRatherThanSent(t *testing.T) {
	ctx := auth.WithIdentity(t.Context(), auth.Identity{
		User:   "alice",
		Groups: []string{"platform", "sre\nX-Evil: 1", ""},
	})
	req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", http.NoBody).WithContext(ctx)

	seen, err := roundTrip(t, req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	groups := seen.Header.Values(transport.ImpersonateGroupHeader)
	if len(groups) != 1 || groups[0] != "platform" {
		t.Fatalf("group headers = %v, want only the one a header can carry", groups)
	}
}
