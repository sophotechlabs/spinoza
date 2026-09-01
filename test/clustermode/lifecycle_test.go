//go:build clustermode

package clustermode

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestTheProviderCanEndASessionFromItsOwnSide(t *testing.T) {
	deploy(t, oidcValues())
	endProviderSessions(t, "bob")
	bob := signIn(t, "bob")
	if !whoami(t, bob).Authenticated {
		t.Fatal("bob did not sign in")
	}

	endProviderSessions(t, "bob")

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if !whoami(t, bob).Authenticated {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("the session the provider ended still worked")
}

func TestALogoutTokenNamingNoSessionLeavesTheCookieAlone(t *testing.T) {
	values := oidcValues()
	values["auth.oidc.clientID"] = "spinoza-nosid"
	deploy(t, values)
	bob := signIn(t, "bob")
	if !whoami(t, bob).Authenticated {
		t.Fatal("bob did not sign in")
	}

	endProviderSessions(t, "bob")

	time.Sleep(5 * time.Second)
	if !whoami(t, bob).Authenticated {
		t.Fatal("a logout naming only a subject ended a session spinoza could not identify")
	}
	logs := kubectl(t, "-n", namespace, "logs", "deployment/"+release, "--tail=200")
	if !strings.Contains(logs, "spinoza cannot map to one session") {
		t.Fatalf("spinoza did not say it could not act on the logout:\n%s", truncate(logs))
	}
}

func TestSettingsSurviveThePodBeingReplaced(t *testing.T) {
	values := oidcValues()
	values["persistence.enabled"] = "true"
	deploy(t, values)
	alice := signIn(t, "alice")

	status, message := put(t, alice, "/api/settings", `{"values":{"spinoza.theme.v1":"borg"}}`)
	if status != http.StatusOK {
		t.Fatalf("writing settings gave %d: %s", status, message)
	}

	kubectl(t, "-n", namespace, "rollout", "restart", "deployment/"+release)
	kubectl(t, "-n", namespace, "rollout", "status", "deployment/"+release, "--timeout="+deployWait)
	waitEndpoints(t)
	waitReachable(t)
	waitServing(t, "oidc")

	_, after := read(t, signIn(t, "alice"), "/api/settings")
	if !strings.Contains(after, "borg") {
		t.Fatalf("settings did not survive the restart: %s", truncate(after))
	}
}

func TestTheTimelineSurvivesThePodBeingReplaced(t *testing.T) {
	values := baseValues()
	values["auth.mode"] = "none"
	values["auth.allowAnonymous"] = "true"
	values["persistence.enabled"] = "true"
	deploy(t, values)
	held := anonymous(t)

	status, body := read(t, held, "/api/clusters")
	if status != http.StatusOK {
		t.Fatalf("reading clusters gave %d: %s", status, truncate(body))
	}
	var before api.ClusterList
	decodeErr := json.Unmarshal([]byte(body), &before)
	if decodeErr != nil {
		t.Fatalf("decoding clusters: %v", decodeErr)
	}
	if len(before.Clusters) != 1 {
		t.Fatalf("clusters = %+v, want the one served cluster", before.Clusters)
	}
	id := before.Clusters[0].ID
	path := "/api/clusters/timeline?cluster=" + url.QueryEscape(id) + "&kinds=workloads"
	status, body = post(t, held, path)
	if status != http.StatusOK {
		t.Fatalf("enabling the timeline gave %d: %s", status, truncate(body))
	}

	kubectl(t, "-n", namespace, "rollout", "restart", "deployment/"+release)
	kubectl(t, "-n", namespace, "rollout", "status", "deployment/"+release, "--timeout="+deployWait)
	waitEndpoints(t)
	waitReachable(t)
	waitServing(t, "none")

	held = anonymous(t)
	status, body = read(t, held, "/api/clusters")
	if status != http.StatusOK {
		t.Fatalf("reading clusters after restart gave %d: %s", status, truncate(body))
	}
	var after api.ClusterList
	decodeErr = json.Unmarshal([]byte(body), &after)
	if decodeErr != nil {
		t.Fatalf("decoding clusters after restart: %v", decodeErr)
	}
	if len(after.Clusters) != 1 || after.Clusters[0].ID != id {
		t.Fatalf("clusters after restart = %+v, want %q", after.Clusters, id)
	}
	if after.Clusters[0].Timeline != "workloads" {
		t.Fatalf("timeline after restart = %q, want workloads", after.Clusters[0].Timeline)
	}

	probe := fmt.Sprintf("timeline-restart-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = maybeKubectl(t, "-n", "default", "delete", "deployment", probe, "--ignore-not-found=true")
	})
	kubectl(t, "-n", "default", "create", "deployment", probe, "--image=nginx:1.27-alpine")
	historyPath := "/api/history?source=change&cluster=" + url.QueryEscape(id) + "&limit=200"
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		status, body = read(t, held, historyPath)
		if status == http.StatusOK {
			var history api.History
			decodeErr = json.Unmarshal([]byte(body), &history)
			if decodeErr != nil {
				t.Fatalf("decoding history: %v", decodeErr)
			}
			for _, entry := range history.Entries {
				if entry.Name == probe {
					return
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("the resumed timeline did not record %q: %s", probe, truncate(body))
}

func TestTheDesktopBuildRefusesToServeACluster(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "spinoza-desktop")
	build := exec.CommandContext(t.Context(), "go", "build", "-tags", "desktop", "-o", binary, "../..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the desktop binary: %v\n%s", err, out)
	}

	run := exec.CommandContext(t.Context(), binary, "--cluster-mode", "--public-url", base)
	out, err := run.CombinedOutput()
	if err == nil {
		t.Fatalf("the desktop app served a cluster:\n%s", out)
	}
	if !strings.Contains(string(out), "cannot serve a cluster") {
		t.Fatalf("it refused with %q, want it to say why", out)
	}
}

func endProviderSessions(t *testing.T, user string) {
	t.Helper()
	script := `
import json, urllib.parse, urllib.request
KC = "http://keycloak:8080"
def call(path, token=None, data=None, method=None, form=False):
    body = None
    headers = {}
    if data is not None:
        if form:
            body = urllib.parse.urlencode(data).encode()
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        else:
            body = json.dumps(data).encode()
            headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(KC + path, data=body, headers=headers, method=method)
    with urllib.request.urlopen(req) as resp:
        raw = resp.read()
        return resp.status, (json.loads(raw) if raw else None)
_, tok = call("/realms/master/protocol/openid-connect/token", data={
    "client_id": "admin-cli", "username": "admin", "password": "admin", "grant_type": "password"}, form=True)
token = tok["access_token"]
_, users = call("/admin/realms/spinoza/users?username=USER", token)
status, _ = call("/admin/realms/spinoza/users/%s/logout" % users[0]["id"], token, data={}, method="POST")
print("logout", status)
`
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.ReplaceAll(script, "USER", user)))
	pod := fmt.Sprintf("kckick-%s-%d", user, time.Now().UnixNano()%100000)
	out := kubectl(t, "-n", "keycloak", "run", pod, "--rm", "-i", "--restart=Never",
		"--image=python:3.13-alpine", "--command", "--",
		"sh", "-c", "echo "+encoded+" | base64 -d | python3 -")
	if !strings.Contains(out, "logout 204") {
		t.Fatalf("the provider would not end %s's session:\n%s", user, out)
	}
}

func put(t *testing.T, held *http.Client, path, payload string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, base+path, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("Origin", base)
	req.Header.Set("Content-Type", "application/json")
	resp, doErr := held.Do(req)
	if doErr != nil {
		t.Fatalf("PUT %s: %v", path, doErr)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, body(t, resp)
}
