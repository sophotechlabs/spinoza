package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

func upgradeBody() map[string]string {
	return map[string]string{
		"namespace": "demo",
		"name":      "podinfo",
		"chart":     "podinfo",
		"repo":      "https://charts.example.com",
		"version":   "6.10.0",
		"values":    "replicaCount: 2\n",
	}
}

func postUpgrade(t *testing.T, url string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return doRequest(t, http.MethodPost, url, bytes.NewReader(encoded))
}

func TestTheUpgradeEndpointDecodesTheBody(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "upgrade", Message: "upgraded"}}
	ts := stubbedServer(t, backend)

	resp, body := postUpgrade(t, ts.URL+"/api/helm/upgrade", upgradeBody())

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(backend.upgrades) != 1 {
		t.Fatalf("upgrades = %d, want one", len(backend.upgrades))
	}
	got := backend.upgrades[0]
	want := helm.UpgradeRequest{
		Namespace: "demo",
		Name:      "podinfo",
		Chart:     "podinfo",
		Version:   "6.10.0",
		RepoURL:   "https://charts.example.com",
		Values:    "replicaCount: 2\n",
	}
	if got != want {
		t.Fatalf("request = %+v, want %+v", got, want)
	}
}

func TestTheUpgradeEndpointPassesDryRunThrough(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "upgrade", DryRun: true, Manifest: "kind: ConfigMap\n"}}
	ts := stubbedServer(t, backend)

	resp, body := postUpgrade(t, ts.URL+"/api/helm/upgrade?dryRun=true", upgradeBody())

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if !backend.upgrades[0].DryRun {
		t.Fatal("dryRun did not reach the backend")
	}
	var got api.HelmActionResult
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if !got.DryRun || got.Manifest == "" {
		t.Fatalf("result = %+v, want the rendered manifest back", got)
	}
}

func TestTheUpgradeEndpointDetectsAnOCIRepo(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "upgrade"}}
	ts := stubbedServer(t, backend)
	body := upgradeBody()
	body["repo"] = "oci://ghcr.io/acme/charts"

	resp, out := postUpgrade(t, ts.URL+"/api/helm/upgrade", body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, out)
	}
	if !backend.upgrades[0].OCI {
		t.Fatal("an oci:// repo was not marked as oci")
	}
}

func TestTheUpgradeEndpointRefusesAnIncompleteBody(t *testing.T) {
	backend := &stubViews{}
	ts := stubbedServer(t, backend)

	for _, missing := range []string{"namespace", "name", "chart", "repo", "version"} {
		t.Run(missing, func(t *testing.T) {
			body := upgradeBody()
			delete(body, missing)
			resp, out := postUpgrade(t, ts.URL+"/api/helm/upgrade", body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode, out)
			}
		})
	}
	if len(backend.upgrades) != 0 {
		t.Fatal("an incomplete request reached the backend")
	}
}

func TestTheUpgradeEndpointRefusesRubbishJSON(t *testing.T) {
	backend := &stubViews{}
	ts := stubbedServer(t, backend)

	resp, out := doRequest(t, http.MethodPost, ts.URL+"/api/helm/upgrade", strings.NewReader("not json"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, out)
	}
	if len(backend.upgrades) != 0 {
		t.Fatal("rubbish reached the backend")
	}
}

func TestTheUpgradeEndpointRefusesAnOversizedBody(t *testing.T) {
	backend := &stubViews{}
	ts := stubbedServer(t, backend)
	body := upgradeBody()
	body["values"] = strings.Repeat("a", (4<<20)+1)

	resp, _ := postUpgrade(t, ts.URL+"/api/helm/upgrade", body)

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
}

func TestTheUpgradeEndpointReportsAFluxOwnedRelease(t *testing.T) {
	backend := &stubViews{actionErr: fmt.Errorf("%w: change the helmrelease object demo/podinfo in git instead", helm.ErrFluxManaged)}
	ts := stubbedServer(t, backend)

	resp, out := postUpgrade(t, ts.URL+"/api/helm/upgrade", upgradeBody())

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", resp.StatusCode, out)
	}
}

func TestAMissingReleaseAnswers404(t *testing.T) {
	backend := &stubViews{actionErr: fmt.Errorf("%w: demo/podinfo", helm.ErrNoRelease)}
	ts := stubbedServer(t, backend)

	resp, _ := postUpgrade(t, ts.URL+"/api/helm/upgrade", upgradeBody())

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAProtectedClusterWantsTheReleaseNameTyped(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "upgrade"}}
	ts := protectedServer(t, backend)

	refused, out := postUpgrade(t, ts.URL+"/api/helm/upgrade", upgradeBody())
	if refused.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412: %s", refused.StatusCode, out)
	}
	if len(backend.upgrades) != 0 {
		t.Fatal("an unconfirmed upgrade reached the backend")
	}

	confirmed, out := postUpgrade(t, ts.URL+"/api/helm/upgrade?confirm=podinfo", upgradeBody())
	if confirmed.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the typed name accepted: %s", confirmed.StatusCode, out)
	}
}

func TestADryRunNeedsNoConfirmationOnAProtectedCluster(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "upgrade", DryRun: true}}
	ts := protectedServer(t, backend)

	resp, out := postUpgrade(t, ts.URL+"/api/helm/upgrade?dryRun=true", upgradeBody())

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want a dry run allowed without typing the name: %s", resp.StatusCode, out)
	}
}

func TestTheVersionsEndpointServesTheList(t *testing.T) {
	backend := &stubViews{versions: api.HelmChartVersions{
		Chart: "podinfo",
		Repos: []api.HelmRepoVersions{
			{Name: "podinfo", URL: "https://stefanprodan.github.io/podinfo", Versions: []string{"6.15.1", "6.14.0"}},
		},
	}}
	ts := stubbedServer(t, backend)

	var got api.HelmChartVersions
	resp := getJSON(t, ts.URL+"/api/helm/versions?chart=podinfo", &got)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(backend.versionsAsked) != 1 || backend.versionsAsked[0] != "podinfo" {
		t.Fatalf("asked = %v", backend.versionsAsked)
	}
	if len(got.Repos) != 1 || got.Repos[0].Versions[0] != "6.15.1" {
		t.Fatalf("got = %+v", got)
	}
}

func TestTheVersionsEndpointRequiresAChart(t *testing.T) {
	backend := &stubViews{}
	ts := stubbedServer(t, backend)

	resp := getJSON(t, ts.URL+"/api/helm/versions", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(backend.versionsAsked) != 0 {
		t.Fatal("a chartless request reached the backend")
	}
}

func TestTheVersionsEndpointReportsABackendFailure(t *testing.T) {
	backend := &stubViews{helmErr: fmt.Errorf("%w: helm is not wired up", api.ErrInternal)}
	ts := stubbedServer(t, backend)

	resp := getJSON(t, ts.URL+"/api/helm/versions?chart=podinfo", nil)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}
