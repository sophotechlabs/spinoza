package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/helm"
)

func TestTheHistoryEndpointReportsAMissingRelease(t *testing.T) {
	backend := &stubViews{detailErr: fmt.Errorf("%w: demo/ghost", helm.ErrNoRelease)}
	ts := stubbedServer(t, backend)

	resp := getJSON(t, ts.URL+"/api/helm/history?namespace=demo&name=ghost", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
