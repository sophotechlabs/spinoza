package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type refusingKindList struct {
	notStubbed
}

func (r refusingKindList) ListKind(context.Context, api.ObjectRef) ([]*unstructured.Unstructured, error) {
	return nil, errors.New("deployments are forbidden here")
}

func TestKindComparisonReportsANearSideListFailure(t *testing.T) {
	cluster := &stubBackendCluster{backend: refusingKindList{notStubbed{t: t}}}
	srv := New(cluster, testAssets(), testToken)
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/compare/kind?group=apps&version=v1&resource=deployments&against=p-mk2",
		http.NoBody,
	)
	recorded := httptest.NewRecorder()

	srv.compareKind(recorded, req)

	if recorded.Code == http.StatusOK {
		t.Fatalf("status = %d, want the failed local list reported", recorded.Code)
	}
	if !strings.Contains(recorded.Body.String(), "deployments are forbidden here") {
		t.Fatalf("body = %q, want the local list failure", recorded.Body.String())
	}
}
