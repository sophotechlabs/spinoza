package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func protect(t *testing.T, ts *httptest.Server) {
	t.Helper()
	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/protection?protected=true", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("protect: %d %s", resp.StatusCode, body)
	}
}

func TestDeletingOnAProtectedClusterNeedsTheNameTyped(t *testing.T) {
	ts := inspectServer(t, newPod())
	protect(t, ts)

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/object"+objectQuery, nil)

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412", resp.StatusCode)
	}
	if !strings.Contains(string(body), "protected") || !strings.Contains(string(body), "web") {
		t.Fatalf("body = %s, want the rule and the name to type", body)
	}

	survived, _ := doRequest(t, http.MethodGet, ts.URL+"/api/object"+objectQuery, nil)
	if survived.StatusCode != http.StatusOK {
		t.Fatal("the object was deleted anyway")
	}
}

func TestDeletingGoesAheadOnceTheNameMatches(t *testing.T) {
	ts := inspectServer(t, newPod())
	protect(t, ts)

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/object"+objectQuery+"&confirm=web", nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.StatusCode, body)
	}
}

func TestTheTypedNameHasToBeTheRightOne(t *testing.T) {
	ts := inspectServer(t, newPod())
	protect(t, ts)

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/object"+objectQuery+"&confirm=wed", nil)

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412 for a near miss", resp.StatusCode)
	}
}

func TestAnUnprotectedClusterAsksForNothing(t *testing.T) {
	ts := inspectServer(t, newPod())

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/object"+objectQuery, nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.StatusCode, body)
	}
}

func TestDrainingAProtectedClusterNeedsTheNodeNameTyped(t *testing.T) {
	ts, _ := actionServer(t, nil, actionNode())
	protect(t, ts)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/action"+nodeQuery+"&action=drain", nil)

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412", resp.StatusCode)
	}
}

func TestPlanningADrainNeedsNoConfirmation(t *testing.T) {
	ts, _ := actionServer(t, nil, actionNode())
	protect(t, ts)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+nodeQuery+"&action=drain&dryRun=true", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the preview to run: %s", resp.StatusCode, body)
	}
}

func TestScalingToZeroOnAProtectedClusterNeedsTheNameTyped(t *testing.T) {
	ts, _ := actionServer(t, nil, actionDeployment())
	protect(t, ts)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/action"+deploymentQuery+"&action=scale&replicas=0", nil)

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; scaling to zero is an outage", resp.StatusCode)
	}
}

func TestScalingUpNeedsNoConfirmation(t *testing.T) {
	ts, _ := actionServer(t, nil, actionDeployment())
	protect(t, ts)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+deploymentQuery+"&action=scale&replicas=3", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want a routine scale to go through: %s", resp.StatusCode, body)
	}
}

func TestRestartingNeedsNoConfirmation(t *testing.T) {
	ts, _ := actionServer(t, nil, actionDeployment())
	protect(t, ts)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/action"+deploymentQuery+"&action=restart", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
}

func TestUninstallingAReleaseOnAProtectedClusterNeedsTheNameTyped(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "uninstall", Message: "gone"}}
	ts := protectedServer(t, backend)

	resp := post(t, ts.URL+"/api/helm/action?namespace=demo&name=podinfo&action=uninstall")

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412", resp.StatusCode)
	}
	if len(backend.uninstalls) != 0 {
		t.Fatalf("uninstalls = %v, want none", backend.uninstalls)
	}
}

func TestUninstallingGoesAheadOnceTheNameMatches(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "uninstall", Message: "gone"}}
	ts := protectedServer(t, backend)

	resp := post(t, ts.URL+"/api/helm/action?namespace=demo&name=podinfo&action=uninstall&confirm=podinfo")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestRollingBackAReleaseOnAProtectedClusterNeedsTheNameTyped(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "rollback", Message: "done", Revision: 2}}
	ts := protectedServer(t, backend)

	resp := post(t, ts.URL+"/api/helm/action?namespace=demo&name=podinfo&action=rollback&revision=2")

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412", resp.StatusCode)
	}
	if len(backend.rollbacks) != 0 {
		t.Fatalf("rollbacks = %v, want none", backend.rollbacks)
	}
}

func TestApplyingOnAProtectedClusterNeedsTheNameTyped(t *testing.T) {
	ts := inspectServer(t, newPod())
	protect(t, ts)
	doc := strings.NewReader("apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n  namespace: flux-system\n  resourceVersion: \"7\"\n")

	resp, body := doRequest(t, http.MethodPut, ts.URL+"/api/object"+objectQuery, doc)

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; apply can take a workload down as thoroughly as delete", resp.StatusCode)
	}
	if !strings.Contains(string(body), "protected") || !strings.Contains(string(body), "web") {
		t.Fatalf("body = %s, want the rule and the name to type", body)
	}
}

func TestApplyingGoesAheadOnceTheNameMatches(t *testing.T) {
	ts := inspectServer(t, newPod())
	protect(t, ts)
	doc := strings.NewReader("apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n  namespace: flux-system\n  resourceVersion: \"7\"\n")

	resp, body := doRequest(t, http.MethodPut, ts.URL+"/api/object"+objectQuery+"&confirm=web", doc)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
}

// interactive access is deliberately outside the protection gate: it is not a
// one-click irreversible act, and typing the object name before every shell
// would make the feature unusable. This test records that decision.

func TestInteractiveAccessIsNotGatedByProtection(t *testing.T) {
	ts := inspectServer(t, newPod())
	protect(t, ts)

	support, _ := doRequest(t, http.MethodGet, ts.URL+"/api/exec/support?namespace=flux-system&pod=web", nil)
	if support.StatusCode == http.StatusPreconditionFailed {
		t.Fatal("exec support asked for a typed name; interactive access is meant to stay open")
	}

	debug, _ := doRequest(t, http.MethodGet, ts.URL+"/api/debug/support?namespace=flux-system&pod=web", nil)
	if debug.StatusCode == http.StatusPreconditionFailed {
		t.Fatal("debug support asked for a typed name; interactive access is meant to stay open")
	}
}
