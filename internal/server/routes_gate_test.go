package server

import (
	"net/http"
	"strings"
	"testing"
)

const noBody = ""

const helmDoc = `{"namespace":"demo","name":"podinfo","chart":"podinfo",` +
	`"repo":"https://stefanprodan.github.io/podinfo","version":"6.9.2"}`

type routeProbe struct {
	method  string
	path    string
	query   string
	confirm string
	body    string
}

func writeProbes() []routeProbe {
	return []routeProbe{
		{
			http.MethodPost, "/api/action",
			"?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web",
			"web", noBody,
		},
		{
			http.MethodPut, "/api/object",
			"?version=v1&resource=configmaps&namespace=demo&name=old",
			"old", "{}",
		},
		{
			http.MethodDelete, "/api/object",
			"?version=v1&resource=configmaps&namespace=demo&name=old",
			"old", noBody,
		},
		{
			http.MethodPost, "/api/flux/action",
			"?action=reconcile&group=kustomize.toolkit.fluxcd.io&version=v1" +
				"&resource=kustomizations&namespace=flux-system&name=apps",
			"apps", noBody,
		},
		{
			http.MethodPost, "/api/argocd/action",
			"?action=sync&group=argoproj.io&version=v1alpha1" +
				"&resource=applications&namespace=argocd&name=shop",
			"shop", noBody,
		},
		{http.MethodPost, "/api/helm/action", "?action=uninstall&namespace=demo&name=podinfo", "podinfo", noBody},
		{http.MethodPost, "/api/helm/upgrade", "", "podinfo", helmDoc},
		{http.MethodPost, "/api/helm/install", "", "podinfo", helmDoc},
		{http.MethodPost, "/api/debug", "?namespace=demo&pod=probe&profile=general", "probe", noBody},
		{http.MethodGet, "/api/nodeshell", "?node=p-mk1", "p-mk1", noBody},
	}
}

func (p routeProbe) key() string {
	return routeKey(p.method, p.path)
}

func everyWritingRoute(t *testing.T) []routeProbe {
	t.Helper()
	probes := map[string]routeProbe{}
	for _, probe := range writeProbes() {
		probes[probe.key()] = probe
	}
	srv := New(&stubBackendCluster{}, testAssets(), testToken)
	srv.UseClusterAuth(ClusterAuth{})
	served := map[string]bool{}
	ordered := make([]routeProbe, 0, len(probes))
	for _, entry := range srv.allRoutes() {
		route := routeKey(entry.method, entry.path)
		served[route] = true
		if !entry.writes {
			continue
		}
		probe, listed := probes[route]
		if !listed {
			t.Errorf("%s changes a cluster but no probe here drives it; add one", route)
			continue
		}
		ordered = append(ordered, probe)
	}
	for route := range probes {
		if !served[route] {
			t.Errorf("%s is probed here but the router no longer serves it", route)
		}
	}
	return ordered
}

func (p routeProbe) send(t *testing.T, base, confirm string) (*http.Response, []byte) {
	t.Helper()
	url := base + p.path + p.query
	if confirm != "" {
		url += p.join() + "confirm=" + confirm
	}
	if p.body == noBody {
		return doRequest(t, p.method, url, http.NoBody)
	}
	return doRequest(t, p.method, url, strings.NewReader(p.body))
}

func (p routeProbe) join() string {
	if p.query == "" {
		return "?"
	}
	return "&"
}

func TestEveryMutatingRouteRefusesAProtectedCluster(t *testing.T) {
	for _, probe := range everyWritingRoute(t) {
		t.Run(probe.key(), func(t *testing.T) {
			ts := protectedServer(t, notStubbed{t: t})

			resp, body := probe.send(t, ts.URL, "")

			if resp.StatusCode != http.StatusPreconditionFailed {
				t.Fatalf("status = %d, want 412 before anything reaches the cluster: %s", resp.StatusCode, body)
			}
			if !strings.Contains(string(body), "protected") {
				t.Fatalf("body = %s, want the rule stated", body)
			}
			if !strings.Contains(string(body), probe.confirm) {
				t.Fatalf("body = %s, want %q as the name to type", body, probe.confirm)
			}
		})
	}
}

func TestEveryMutatingRouteGoesThroughOnceConfirmed(t *testing.T) {
	for _, probe := range everyWritingRoute(t) {
		t.Run(probe.key(), func(t *testing.T) {
			ts := protectedServer(t, &writingBackend{})

			resp, body := probe.send(t, ts.URL, probe.confirm)

			if resp.StatusCode == http.StatusPreconditionFailed {
				t.Fatalf("the typed name was refused as well; the gate blocks everything: %s", body)
			}
		})
	}
}
