//go:build clustermode

package clustermode

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestOnePublishedReleaseCanBeUpgradedAndRolledBack(t *testing.T) {
	previous := os.Getenv("SPINOZA_CM_PREVIOUS_VERSION")
	current := os.Getenv("SPINOZA_CM_CURRENT_VERSION")
	if previous == "" && current == "" {
		t.Skip("published release versions are not set")
	}
	if previous == "" || current == "" {
		t.Fatal("both SPINOZA_CM_PREVIOUS_VERSION and SPINOZA_CM_CURRENT_VERSION are required")
	}
	if previous == current {
		t.Fatalf("previous and current versions are both %q", current)
	}
	if chartReference() == chart {
		t.Fatal("SPINOZA_CM_CHART must name the published OCI chart")
	}
	if os.Getenv("SPINOZA_CM_USE_CHART_IMAGE") != "1" {
		t.Fatal("SPINOZA_CM_USE_CHART_IMAGE must be 1 for a published release test")
	}

	removeRealProxy(t)
	_, _ = maybeHelm(t, "--kube-context", context1(t), "uninstall", release, "--namespace", namespace)
	_, _ = maybeKubectl(t, "-n", namespace, "delete", "persistentvolumeclaim", release,
		"--ignore-not-found=true", "--wait=true")

	values := oidcValues()
	values["persistence.enabled"] = "true"
	deployVersion(t, values, previous)
	requireReleaseImage(t, previous)
	clusterID := seedPublishedReleaseState(t)
	requirePublishedReleaseState(t, clusterID)

	deployVersion(t, values, current)
	requireReleaseImage(t, current)
	requirePublishedReleaseState(t, clusterID)

	run(t, "helm", "--kube-context", context1(t), "rollback", release, "1",
		"--namespace", namespace, "--wait", "--timeout", deployWait)
	kubectl(t, "-n", namespace, "rollout", "status", "deployment/"+release, "--timeout="+deployWait)
	waitEndpoints(t)
	waitReachable(t)
	waitServing(t, "oidc")
	requireReleaseImage(t, previous)
	requirePublishedReleaseState(t, clusterID)
}

func seedPublishedReleaseState(t *testing.T) string {
	t.Helper()
	alice := signIn(t, "alice")
	bob := signIn(t, "bob")
	status, message := put(t, alice, "/api/settings", `{"values":{"spinoza.theme.v1":"borg"}}`)
	if status != http.StatusOK {
		t.Fatalf("writing alice's settings gave %d: %s", status, message)
	}
	status, message = put(t, bob, "/api/settings", `{"values":{"spinoza.theme.v1":"matrix"}}`)
	if status != http.StatusOK {
		t.Fatalf("writing bob's settings gave %d: %s", status, message)
	}

	clusterID := servedClusterID(t, alice)
	path := "/api/clusters/timeline?cluster=" + url.QueryEscape(clusterID) + "&kinds=workloads"
	status, message = post(t, alice, path)
	if status != http.StatusOK {
		t.Fatalf("enabling the timeline gave %d: %s", status, message)
	}

	status, message = post(t, bob, scaleTo("payments", "web", 2))
	if status != http.StatusOK {
		t.Fatalf("the persisted impersonated write gave %d: %s", status, message)
	}
	defer func() {
		_, _ = maybeKubectl(t, "-n", "payments", "scale", "deployment/web", "--replicas=1")
	}()

	waitForHistory(t, alice, api.HistoryChange, "web")
	waitForHistory(t, alice, api.HistoryAction, "web")
	return clusterID
}

func requirePublishedReleaseState(t *testing.T, clusterID string) {
	t.Helper()
	alice := signIn(t, "alice")
	bob := signIn(t, "bob")

	_, aliceSettings := read(t, alice, "/api/settings")
	if !strings.Contains(aliceSettings, "borg") || strings.Contains(aliceSettings, "matrix") {
		t.Fatalf("alice's settings = %s", truncate(aliceSettings))
	}
	_, bobSettings := read(t, bob, "/api/settings")
	if !strings.Contains(bobSettings, "matrix") || strings.Contains(bobSettings, "borg") {
		t.Fatalf("bob's settings = %s", truncate(bobSettings))
	}

	status, body := read(t, alice, "/api/clusters")
	if status != http.StatusOK {
		t.Fatalf("reading clusters gave %d: %s", status, truncate(body))
	}
	var clusters api.ClusterList
	decodeErr := json.Unmarshal([]byte(body), &clusters)
	if decodeErr != nil {
		t.Fatalf("decoding clusters: %v", decodeErr)
	}
	if len(clusters.Clusters) != 1 {
		t.Fatalf("clusters = %+v, want one", clusters.Clusters)
	}
	if clusters.Clusters[0].ID != clusterID || clusters.Clusters[0].Timeline != "workloads" {
		t.Fatalf("cluster = %+v, want %q with workload history", clusters.Clusters[0], clusterID)
	}

	waitForHistory(t, alice, api.HistoryChange, "web")
	waitForHistory(t, alice, api.HistoryAction, "web")
	status, message := post(t, bob, restart("payments", "web"))
	if status != http.StatusOK {
		t.Fatalf("an impersonated write after the lifecycle step gave %d: %s", status, message)
	}
	status, message = post(t, bob, restart("default", "other"))
	if status != http.StatusForbidden {
		t.Fatalf("a forbidden write after the lifecycle step gave %d: %s", status, message)
	}
	if !strings.Contains(messageOf(t, message), `User "bob"`) {
		t.Fatalf("the cluster refused %q, want it to name bob", message)
	}
}

func servedClusterID(t *testing.T, held *http.Client) string {
	t.Helper()
	status, body := read(t, held, "/api/clusters")
	if status != http.StatusOK {
		t.Fatalf("reading clusters gave %d: %s", status, truncate(body))
	}
	var clusters api.ClusterList
	decodeErr := json.Unmarshal([]byte(body), &clusters)
	if decodeErr != nil {
		t.Fatalf("decoding clusters: %v", decodeErr)
	}
	if len(clusters.Clusters) != 1 {
		t.Fatalf("clusters = %+v, want one", clusters.Clusters)
	}
	return clusters.Clusters[0].ID
}

func waitForHistory(t *testing.T, held *http.Client, source, name string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	path := "/api/history?source=" + url.QueryEscape(source) + "&limit=200"
	last := ""
	for time.Now().Before(deadline) {
		status, body := read(t, held, path)
		last = body
		if status == http.StatusOK {
			var history api.History
			decodeErr := json.Unmarshal([]byte(body), &history)
			if decodeErr != nil {
				t.Fatalf("decoding %s history: %v", source, decodeErr)
			}
			for _, entry := range history.Entries {
				if entry.Name == name {
					return
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("%s history did not retain %q: %s", source, name, truncate(last))
}

func requireReleaseImage(t *testing.T, version string) {
	t.Helper()
	image := strings.TrimSpace(kubectl(t, "-n", namespace, "get", "deployment/"+release,
		"-o", "jsonpath={.spec.template.spec.containers[0].image}"))
	if !strings.HasSuffix(image, ":"+version) {
		t.Fatalf("image = %q, want version %q", image, version)
	}
}
