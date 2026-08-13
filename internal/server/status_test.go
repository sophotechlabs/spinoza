package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func podsResource() schema.GroupResource {
	return schema.GroupResource{Resource: "pods"}
}

func TestStatusForMapsEachOutcome(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"internal sentinel", fmt.Errorf("action: %w", api.ErrInternal), http.StatusInternalServerError},
		{"oversized body", &http.MaxBytesError{Limit: 1}, http.StatusRequestEntityTooLarge},
		{"invalid uid", inspect.ErrInvalidUID, http.StatusBadRequest},
		{"no schema", jsonschema.ErrNoSchema, http.StatusNotFound},
		{"deadline exceeded", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"canceled", context.Canceled, http.StatusServiceUnavailable},
		{"not found", apierrors.NewNotFound(podsResource(), "web"), http.StatusNotFound},
		{"conflict", apierrors.NewConflict(podsResource(), "web", errors.New("newer")), http.StatusConflict},
		{"unauthorized", apierrors.NewUnauthorized("who is this"), http.StatusUnauthorized},
		{"forbidden", apierrors.NewForbidden(podsResource(), "web", errors.New("no")), http.StatusForbidden},
		{"bad request", apierrors.NewBadRequest("no such field"), http.StatusUnprocessableEntity},
		{"anything else", errors.New("unknown resource apps/v1/widgets"), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusFor(tc.err); got != tc.want {
				t.Fatalf("statusFor = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestStatusForUpstreamFailures(t *testing.T) {
	cases := map[string]error{
		"prometheus is unavailable": prom.ErrUnavailable,
		"wrapped prometheus":        fmt.Errorf("metrics: %w", prom.ErrUnavailable),
		"cache never synced":        resources.ErrNotSynced,
		"wrapped sync failure":      fmt.Errorf("subscribe: %w", resources.ErrNotSynced),
		"apiserver timeout":         apierrors.NewTimeoutError("gave up", 1),
		"server timeout":            apierrors.NewServerTimeout(podsResource(), "list", 1),
		"service unavailable":       apierrors.NewServiceUnavailable("down"),
		"internal error":            apierrors.NewInternalError(errors.New("boom")),
		"too many requests":         apierrors.NewTooManyRequests("slow down", 1),
		"network failure":           &net.OpError{Op: "dial", Err: errors.New("connection refused")},
		"wrapped network failure":   fmt.Errorf("listing: %w", &net.OpError{Op: "dial", Err: errors.New("no route to host")}),
	}
	for name, err := range cases {
		if got := statusFor(err); got != http.StatusServiceUnavailable {
			t.Fatalf("%s: statusFor = %d, want 503", name, got)
		}
	}
}

func TestStatusForCallerMistakesStaysBadRequest(t *testing.T) {
	cases := map[string]error{
		"unknown resource": errors.New("unknown resource apps/v1/widgets"),
		"missing field":    errors.New("version, resource and name are required"),
		"bad range":        errors.New("range must be a duration such as 1h"),
	}
	for name, err := range cases {
		if got := statusFor(err); got != http.StatusBadRequest {
			t.Fatalf("%s: statusFor = %d, want 400", name, got)
		}
	}
}

func TestAForbiddenListKeepsItsStatusThroughTheStatusError(t *testing.T) {
	err := &apierrors.StatusError{ErrStatus: metav1.Status{
		Code:   http.StatusForbidden,
		Reason: metav1.StatusReasonForbidden,
	}}

	if got := statusFor(fmt.Errorf("listing events: %w", err)); got != http.StatusForbidden {
		t.Fatalf("statusFor = %d, want the wrapped reason to survive", got)
	}
}

func eventsServer(t *testing.T, listErr error) *httptest.Server {
	t.Helper()
	eventGVR := schema.GroupVersionResource{Version: "v1", Resource: "events"}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{eventGVR: "EventList"},
	)
	if listErr != nil {
		dyn.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, listErr
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset()})
	ts := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func TestEventsSurfaceAListFailure(t *testing.T) {
	ts := eventsServer(t, apierrors.NewForbidden(
		schema.GroupResource{Resource: "events"}, "", errors.New("no access"),
	))

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/events?namespace=default&uid=6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84", nil)

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 rather than an empty event list: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "no access") {
		t.Fatalf("body = %s, want the reason", body)
	}
}

func TestEventsWithoutAUIDStayEmpty(t *testing.T) {
	ts := eventsServer(t, nil)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/events?namespace=default", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if string(body) != "[]\n" && string(body) != "[]" {
		t.Fatalf("body = %q, want an empty list", body)
	}
}

func TestAnUnreachablePrometheusOutranksTheWrappedApiError(t *testing.T) {
	inner := apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, "prometheus")
	err := fmt.Errorf("%w: probe failed: %w", prom.ErrUnavailable, inner)

	if got := statusFor(err); got != http.StatusServiceUnavailable {
		t.Fatalf("statusFor = %d, want 503; spinoza could not reach Prometheus", got)
	}
}

func TestASyncFailureOutranksTheWrappedApiError(t *testing.T) {
	inner := apierrors.NewForbidden(podsResource(), "", errors.New("nope"))
	err := fmt.Errorf("%w: %w", resources.ErrNotSynced, inner)

	if got := statusFor(err); got != http.StatusServiceUnavailable {
		t.Fatalf("statusFor = %d, want 503", got)
	}
}
