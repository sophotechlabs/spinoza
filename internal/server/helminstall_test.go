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

func installBody() map[string]any {
	return map[string]any{
		"namespace":       "demo",
		"name":            "podinfo",
		"chart":           "podinfo",
		"repo":            "https://charts.example.com",
		"version":         "6.10.0",
		"values":          "replicaCount: 2\n",
		"createNamespace": true,
	}
}

func postInstall(t *testing.T, url string, body map[string]any) (*http.Response, []byte) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return doRequest(t, http.MethodPost, url, bytes.NewReader(encoded))
}

func TestTheInstallEndpointDecodesTheBody(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "install", Message: "installed"}}
	ts := stubbedServer(t, backend)

	resp, body := postInstall(t, ts.URL+"/api/helm/install", installBody())

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(backend.installs) != 1 {
		t.Fatalf("installs = %d, want one", len(backend.installs))
	}
	want := helm.InstallRequest{
		Namespace:       "demo",
		Name:            "podinfo",
		Chart:           "podinfo",
		Version:         "6.10.0",
		RepoURL:         "https://charts.example.com",
		Values:          "replicaCount: 2\n",
		CreateNamespace: true,
	}
	if backend.installs[0] != want {
		t.Fatalf("request = %+v, want %+v", backend.installs[0], want)
	}
}

func TestTheInstallEndpointMarksAnOCIRepo(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "install"}}
	ts := stubbedServer(t, backend)
	body := installBody()
	body["repo"] = "oci://registry.example.com/charts"

	resp, out := postInstall(t, ts.URL+"/api/helm/install", body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, out)
	}
	if !backend.installs[0].OCI {
		t.Fatal("an oci:// repo was not marked as one")
	}
}

func TestTheInstallEndpointPassesDryRunThrough(t *testing.T) {
	backend := &stubViews{action: api.HelmActionResult{Action: "install", DryRun: true, Manifest: "kind: Service\n"}}
	ts := stubbedServer(t, backend)

	resp, body := postInstall(t, ts.URL+"/api/helm/install?dryRun=true", installBody())

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if !backend.installs[0].DryRun {
		t.Fatal("dryRun did not reach the backend")
	}
}

func TestTheInstallEndpointWantsEveryField(t *testing.T) {
	for _, missing := range []string{"namespace", "name", "chart", "repo", "version"} {
		t.Run(missing, func(t *testing.T) {
			backend := &stubViews{}
			ts := stubbedServer(t, backend)
			body := installBody()
			body[missing] = ""

			resp, out := postInstall(t, ts.URL+"/api/helm/install", body)

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", resp.StatusCode, out)
			}
			if len(backend.installs) != 0 {
				t.Fatal("an incomplete request reached the backend")
			}
		})
	}
}

func TestTheInstallEndpointRefusesSomethingThatIsNotJSON(t *testing.T) {
	backend := &stubViews{}
	ts := stubbedServer(t, backend)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/helm/install", strings.NewReader("{"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "must be json") {
		t.Fatalf("body = %s", body)
	}
}

func TestTheChartSearchEndpointPassesTheQuery(t *testing.T) {
	backend := &stubViews{search: api.HelmChartSearch{
		Query: "podinfo",
		Hits:  []api.HelmChartHit{{Chart: "podinfo", Version: "6.10.0", URL: "https://charts.example.com"}},
	}}
	ts := stubbedServer(t, backend)

	var found api.HelmChartSearch
	resp := getJSON(t, ts.URL+"/api/helm/charts?query=podinfo", &found)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(backend.searchAsked) != 1 || backend.searchAsked[0] != "podinfo" {
		t.Fatalf("asked = %v", backend.searchAsked)
	}
	if len(found.Hits) != 1 || found.Hits[0].Chart != "podinfo" {
		t.Fatalf("hits = %+v", found.Hits)
	}
}

func TestTheChartSearchEndpointReportsAFailure(t *testing.T) {
	backend := &stubViews{helmErr: fmt.Errorf("%w: helm is not wired up", api.ErrInternal)}
	ts := stubbedServer(t, backend)

	resp := getJSON(t, ts.URL+"/api/helm/charts?query=podinfo", nil)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestTheChartValuesEndpointNeedsEveryPart(t *testing.T) {
	backend := &stubViews{}
	ts := stubbedServer(t, backend)

	resp := getJSON(t, ts.URL+"/api/helm/values?chart=podinfo", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(backend.valuesAsked) != 0 {
		t.Fatal("an incomplete request reached the backend")
	}
}

func TestTheChartValuesEndpointAsksForTheChart(t *testing.T) {
	backend := &stubViews{chartValues: api.HelmChartValues{Chart: "podinfo", Version: "6.10.0", Values: "replicaCount: 1\n"}}
	ts := stubbedServer(t, backend)

	var found api.HelmChartValues
	resp := getJSON(t, ts.URL+"/api/helm/values?chart=podinfo&repo=oci://registry.example.com/charts&version=6.10.0", &found)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	want := helm.ValuesRequest{
		Chart:   "podinfo",
		Version: "6.10.0",
		RepoURL: "oci://registry.example.com/charts",
		OCI:     true,
	}
	if len(backend.valuesAsked) != 1 || backend.valuesAsked[0] != want {
		t.Fatalf("asked = %+v, want %+v", backend.valuesAsked, want)
	}
	if found.Values != "replicaCount: 1\n" {
		t.Fatalf("values = %q", found.Values)
	}
}

func TestTheChartValuesEndpointReportsAFailure(t *testing.T) {
	backend := &stubViews{helmErr: fmt.Errorf("%w: helm is not wired up", api.ErrInternal)}
	ts := stubbedServer(t, backend)

	resp := getJSON(t, ts.URL+"/api/helm/values?chart=podinfo&repo=https://charts.example.com&version=6.10.0", nil)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
