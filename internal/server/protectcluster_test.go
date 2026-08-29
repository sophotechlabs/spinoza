package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func guardedFleet(t *testing.T, protect string) (*httptest.Server, *fleet) {
	t.Helper()
	srv, held := twoClusters(t, &writingBackend{}, &writingBackend{})
	if protect != "" {
		if err := held.Protect(protect, true); err != nil {
			t.Fatalf("protect: %v", err)
		}
	}
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts, held
}

func scaleToNothing(t *testing.T, ts *httptest.Server, cluster string) int {
	t.Helper()
	asked := ts.URL + "/api/action?action=scale&replicas=0&group=apps&version=v1&resource=deployments" +
		"&namespace=default&name=web&cluster=" + urlValue(cluster)
	resp, _ := doRequest(t, http.MethodPost, asked, nil)
	return resp.StatusCode
}

func TestAProtectedBackgroundClusterStillRefusesAnUnconfirmedWrite(t *testing.T) {
	ts, _ := guardedFleet(t, mk2)

	if status := scaleToNothing(t, ts, mk2); status != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want the protected cluster to refuse; its guard was read off another cluster", status)
	}
}

func TestOneClustersProtectionDoesNotStopAWriteToAnother(t *testing.T) {
	ts, _ := guardedFleet(t, mk1)

	if status := scaleToNothing(t, ts, mk2); status != http.StatusOK {
		t.Fatalf("status = %d, want the unprotected cluster to accept; the active cluster's flag was applied", status)
	}
}

func TestProtectingNamesTheClusterAsked(t *testing.T) {
	ts, held := guardedFleet(t, "")

	doRequest(t, http.MethodPost, ts.URL+"/api/protection?protected=true&cluster="+urlValue(mk2), nil)

	if !held.Protected(mk2) {
		t.Fatal("the cluster that was asked for was not protected")
	}
	if held.Protected(mk1) {
		t.Fatal("the active cluster was protected instead of the one asked for")
	}
}
