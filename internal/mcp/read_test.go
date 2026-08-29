package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/prom"
)

// helpers

func run(t *testing.T, server *Server, name string, args arguments) map[string]any {
	t.Helper()
	found, known := server.tools[name]
	if !known {
		t.Fatalf("%s is not registered", name)
	}
	result, err := found.run(context.Background(), args)
	if err != nil {
		t.Fatalf("%s failed: %v", name, err)
	}
	out, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("%s returned %T, want an object", name, result)
	}
	return out
}

func refuses(t *testing.T, server *Server, name string, args arguments) error {
	t.Helper()
	found, known := server.tools[name]
	if !known {
		t.Fatalf("%s is not registered", name)
	}
	_, err := found.run(context.Background(), args)
	if err == nil {
		t.Fatalf("%s accepted %v", name, args)
	}
	return err
}

func object(kind, namespace, name string) *unstructured.Unstructured {
	item := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": kind}}
	item.SetName(name)
	item.SetNamespace(namespace)
	item.SetCreationTimestamp(metav1.Now())
	return item
}

// the dashboard

func TestTheDashboardJoinsHealthCountsIssuesAndTheAudit(t *testing.T) {
	cluster := &fakeCluster{
		overview: api.ClusterOverview{Version: "v1.30.0", Nodes: api.NodeSummary{Total: 3, Ready: 3}},
		counts:   api.ResourceCounts{Failing: map[string]int{"apps/v1/deployments": 2}},
		queue: api.IssueQueue{Rows: []api.Issue{{
			Severity: api.SeverityFatal,
			Title:    "CrashLoopBackOff",
			Object:   api.ObjectRef{Name: "web"},
		}}},
		report: api.CheckReport{
			Scanned: 12,
			Groups:  []api.CheckGroup{{ID: "image-latest", Findings: []api.CheckFinding{{Detail: "uses :latest"}}}},
		},
	}
	server := serverFor(cluster, Options{Context: "p-mk1"})

	result := run(t, server, "get_dashboard", arguments{})

	if result["kubernetes"] != "v1.30.0" {
		t.Fatalf("kubernetes = %v", result["kubernetes"])
	}
	if result["context"] != "p-mk1" {
		t.Fatalf("context = %v", result["context"])
	}
	if result["issueCount"] != 1 {
		t.Fatalf("issueCount = %v", result["issueCount"])
	}
	if result["auditFound"] != 1 {
		t.Fatalf("auditFound = %v", result["auditFound"])
	}
	lines := as[[]string](t, result["issues"])
	if len(lines) != 1 || !strings.Contains(lines[0], "CrashLoopBackOff") {
		t.Fatalf("issues = %v", lines)
	}
	if as[map[string]int](t, result["failing"])["apps/v1/deployments"] != 2 {
		t.Fatalf("failing = %v", result["failing"])
	}
}

func TestTheDashboardReportsEachFailureOnce(t *testing.T) {
	same := "the apiserver is unreachable"
	cluster := &fakeCluster{
		overview: api.ClusterOverview{Error: same},
		queue:    api.IssueQueue{Error: same},
		report:   api.CheckReport{Error: "checks could not run"},
	}
	server := serverFor(cluster, Options{})

	result := run(t, server, "get_dashboard", arguments{})

	errs := as[[]string](t, result["errors"])
	if len(errs) != 2 {
		t.Fatalf("errors = %v, want the repeat collapsed", errs)
	}
}

// namespaces and kinds

func TestNamespacesComeBackAsRows(t *testing.T) {
	cluster := &fakeCluster{spaces: api.Namespaces{Names: []string{"default", "prod"}}}
	server := serverFor(cluster, Options{})

	result := run(t, server, "list_namespaces", arguments{})

	rows := as[[]map[string]string](t, result["namespaces"])
	if len(rows) != 2 || rows[1]["name"] != "prod" {
		t.Fatalf("namespaces = %v", rows)
	}
}

func TestTheKindListGroupsWhatTheClusterServes(t *testing.T) {
	cluster := &fakeCluster{catalog: catalogOf(
		descriptor("apps", "v1", "deployments", "Deployment"),
		descriptor("", "v1", "pods", "Pod"),
	)}
	server := serverFor(cluster, Options{})

	result := run(t, server, "list_resource_kinds", arguments{})

	kinds := as[map[string][]string](t, result["kinds"])
	if len(kinds["Workloads"]) != 2 {
		t.Fatalf("kinds = %v", kinds)
	}
	if kinds["Workloads"][0] != "deployments.apps" {
		t.Fatalf("a grouped resource reads %q, want it qualified", kinds["Workloads"][0])
	}
	if kinds["Workloads"][1] != "pods" {
		t.Fatalf("a core resource reads %q, want it bare", kinds["Workloads"][1])
	}
}

// listing

func TestListingSummarisesAndCaps(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("", "v1", "pods", "Pod")),
		listed: []*unstructured.Unstructured{
			object("Pod", "prod", "web-0"),
			object("Pod", "prod", "web-1"),
			object("Pod", "prod", "web-2"),
		},
	}
	server := serverFor(cluster, Options{})

	result := run(t, server, "list_resources", arguments{"resource": "pods", "limit": float64(2)})

	if result["total"] != 3 || result["returned"] != 2 {
		t.Fatalf("total/returned = %v/%v, want 3/2", result["total"], result["returned"])
	}
	rows := as[[]map[string]any](t, result["items"])
	if rows[0]["name"] != "web-0" || rows[0]["namespace"] != "prod" {
		t.Fatalf("row = %v", rows[0])
	}
	if _, aged := rows[0]["age"]; !aged {
		t.Fatal("a row carries no age")
	}
}

func TestListingKeepsOnlyTheNamespaceAsked(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("", "v1", "pods", "Pod")),
		listed: []*unstructured.Unstructured{
			object("Pod", "prod", "web-0"),
			object("Pod", "staging", "web-1"),
		},
	}
	server := serverFor(cluster, Options{})

	result := run(t, server, "list_resources", arguments{"resource": "pods", "namespace": "prod"})

	rows := as[[]map[string]any](t, result["items"])
	if len(rows) != 1 || rows[0]["name"] != "web-0" {
		t.Fatalf("items = %v", rows)
	}
}

func TestListingPassesTheApiserverRefusalStraightBack(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("", "v1", "pods", "Pod")),
		listErr: errRefused,
	}
	server := serverFor(cluster, Options{})

	if err := refuses(t, server, "list_resources", arguments{"resource": "pods"}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v, want the apiserver's own", err)
	}
}

func TestAClusterScopedRowCarriesNoNamespace(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("", "v1", "nodes", "Node")),
		listed:  []*unstructured.Unstructured{object("Node", "", "worker-1")},
	}
	server := serverFor(cluster, Options{})

	rows := as[[]map[string]any](t, run(t, server, "list_resources", arguments{"resource": "nodes"})["items"])

	if _, held := rows[0]["namespace"]; held {
		t.Fatalf("a cluster-scoped row carries a namespace: %v", rows[0])
	}
}

// one resource

func TestGettingAResourceCarriesTheShapeAndNotTheSecret(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("", "v1", "secrets", "Secret")),
		detail: api.ObjectDetail{
			Kind:      "Secret",
			Name:      "tls",
			Namespace: "prod",
			Data:      []api.DataEntry{{Key: "password", Value: "hunter2", Bytes: 7}},
			YAML:      "data:\n  password: aHVudGVyMg==\n",
		},
	}
	server := serverFor(cluster, Options{})

	result := run(t, server, "get_resource", arguments{"resource": "secrets", "name": "tls"})

	if result["yaml"] != "" {
		t.Fatalf("the raw document came back for a Secret: %q", result["yaml"])
	}
	keys := as[[]dataKey](t, result["dataKeys"])
	if len(keys) != 1 || keys[0].Key != "password" || keys[0].Bytes != 7 {
		t.Fatalf("dataKeys = %v", keys)
	}
	if !strings.Contains(note(result), "withholds") {
		t.Fatalf("note = %v, want the withholding said out loud", result["note"])
	}
	body := as[string](t, result["yaml"]) + " " + note(result)
	if strings.Contains(body, "hunter2") {
		t.Fatalf("the value leaked: %q", body)
	}
}

func note(result map[string]any) string {
	found, held := result["note"].(string)
	if !held {
		return ""
	}
	return found
}

func TestGettingAnOrdinaryResourceKeepsItsDocument(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("apps", "v1", "deployments", "Deployment")),
		detail: api.ObjectDetail{
			Kind: "Deployment",
			Name: "web",
			YAML: "spec:\n  replicas: 3\n",
		},
	}
	server := serverFor(cluster, Options{})

	result := run(t, server, "get_resource", arguments{"resource": "deployments", "name": "web"})

	if !strings.Contains(as[string](t, result["yaml"]), "replicas: 3") {
		t.Fatalf("yaml = %q", result["yaml"])
	}
	if _, held := result["note"]; held {
		t.Fatal("an ordinary object carries a withholding note")
	}
	if _, held := result["dataKeys"]; held {
		t.Fatal("an object with no data carries a dataKeys list")
	}
}

func TestGettingAResourceWithItsEvents(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("", "v1", "pods", "Pod")),
		detail:  api.ObjectDetail{Kind: "Pod", Name: "web-0", UID: "uid-1"},
		events:  []api.Event{{Reason: "Pulled", Message: "pulled image", Count: 1}},
	}
	server := serverFor(cluster, Options{})

	result := run(t, server, "get_resource", arguments{"resource": "pods", "name": "web-0", "events": true})

	rows := as[[]map[string]any](t, result["events"])
	if len(rows) != 1 || rows[0]["reason"] != "Pulled" {
		t.Fatalf("events = %v", rows)
	}
}

func TestEventsAreOnlyFetchedWhenAsked(t *testing.T) {
	cluster := &fakeCluster{
		catalog:   catalogOf(descriptor("", "v1", "pods", "Pod")),
		detail:    api.ObjectDetail{Kind: "Pod", Name: "web-0"},
		eventsErr: errRefused,
	}
	server := serverFor(cluster, Options{})

	if _, held := run(t, server, "get_resource", arguments{"resource": "pods", "name": "web-0"})["events"]; held {
		t.Fatal("events came back without being asked for")
	}
	if err := refuses(t, server, "get_resource", arguments{"resource": "pods", "name": "web-0", "events": true}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
}

func TestGettingAResourceThatIsNotThere(t *testing.T) {
	cluster := &fakeCluster{
		catalog:   catalogOf(descriptor("", "v1", "pods", "Pod")),
		detailErr: errRefused,
	}
	server := serverFor(cluster, Options{})

	if err := refuses(t, server, "get_resource", arguments{"resource": "pods", "name": "gone"}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
}

// events

func TestEventsAreFoldedByReasonAndMessage(t *testing.T) {
	cluster := &fakeCluster{events: []api.Event{
		{Reason: "BackOff", Message: "restarting", Count: 3, LastSeen: "now"},
		{Reason: "BackOff", Message: "restarting", Count: 4},
		{Reason: "Pulled", Message: "pulled image", Count: 1},
	}}
	server := serverFor(cluster, Options{})

	rows := as[[]map[string]any](t, run(t, server, "get_events", arguments{})["events"])

	if len(rows) != 2 {
		t.Fatalf("events = %v, want the repeat folded", rows)
	}
	if rows[0]["count"] != 7 {
		t.Fatalf("count = %v, want the two counts added", rows[0]["count"])
	}
	if rows[0]["lastSeen"] != "now" {
		t.Fatalf("lastSeen = %v, want the first one's", rows[0]["lastSeen"])
	}
}

func TestAnEventMessageIsScrubbed(t *testing.T) {
	cluster := &fakeCluster{events: []api.Event{
		{Reason: "Failed", Message: "auth failed with password=hunter2000"},
	}}
	server := serverFor(cluster, Options{})

	rows := as[[]map[string]any](t, run(t, server, "get_events", arguments{})["events"])

	if strings.Contains(as[string](t, rows[0]["message"]), "hunter2000") {
		t.Fatalf("an event message leaked a secret: %q", rows[0]["message"])
	}
}

func TestEventsAreCapped(t *testing.T) {
	found := make([]api.Event, 0, 5)
	for index := range 5 {
		found = append(found, api.Event{Reason: "R", Message: string(rune('a' + index))})
	}
	server := serverFor(&fakeCluster{events: found}, Options{})

	rows := as[[]map[string]any](t, run(t, server, "get_events", arguments{"limit": float64(2)})["events"])

	if len(rows) != 2 {
		t.Fatalf("events = %d, want 2", len(rows))
	}
}

func TestEventsPassTheRefusalBack(t *testing.T) {
	server := serverFor(&fakeCluster{eventsErr: errRefused}, Options{})

	if err := refuses(t, server, "get_events", arguments{}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
}

// topology

func TestTopologyCarriesTheFoldCounts(t *testing.T) {
	cluster := &fakeCluster{graph: api.Graph{
		Nodes: []api.GraphNode{
			{ID: "dep", Kind: "Deployment", Name: "web", Contains: 4, Unhealthy: 1, Ready: "False"},
			{ID: "svc", Kind: "Service", Name: "web"},
		},
		Edges: []api.GraphEdge{{From: "svc", To: "dep", Kind: "routes"}},
	}}
	server := serverFor(cluster, Options{})

	result := run(t, server, "get_topology", arguments{"namespace": "prod", "expand": "a,b"})

	nodes := as[[]map[string]any](t, result["nodes"])
	if nodes[0]["folded"] != 4 || nodes[0]["foldedUnhealthy"] != 1 {
		t.Fatalf("node = %v", nodes[0])
	}
	if _, held := nodes[1]["folded"]; held {
		t.Fatal("a node with nothing inside reports a fold count")
	}
	if cluster.lastTopo.Namespace != "prod" {
		t.Fatalf("namespace = %q", cluster.lastTopo.Namespace)
	}
	if len(cluster.lastTopo.Expanded) != 2 {
		t.Fatalf("expanded = %v, want the comma list split", cluster.lastTopo.Expanded)
	}
	if len(as[[]map[string]string](t, result["edges"])) != 1 {
		t.Fatalf("edges = %v", result["edges"])
	}
}

func TestTopologyWithNothingOpenSendsNoExpandList(t *testing.T) {
	cluster := &fakeCluster{}
	server := serverFor(cluster, Options{})

	run(t, server, "get_topology", arguments{})

	if cluster.lastTopo.Expanded != nil {
		t.Fatalf("expanded = %v, want nothing", cluster.lastTopo.Expanded)
	}
}

// logs

func TestPodLogsAreScrubbedAndCapped(t *testing.T) {
	cluster := &fakeCluster{lines: []string{"starting", "password=hunter2000", "ready"}}
	server := serverFor(cluster, Options{LogLines: 2})

	result := run(t, server, "get_pod_logs", arguments{"namespace": "prod", "name": "web-0"})

	lines := as[[]string](t, result["lines"])
	if len(lines) != 2 {
		t.Fatalf("lines = %v, want the cap honored", lines)
	}
	if strings.Contains(lines[1], "hunter2000") {
		t.Fatalf("a log line leaked a secret: %q", lines[1])
	}
	if cluster.lastLogs.Namespace != "prod" || cluster.lastLogs.Name != "web-0" {
		t.Fatalf("request = %+v", cluster.lastLogs)
	}
}

func TestPodLogsNeedBothANamespaceAndAName(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	if err := refuses(t, server, "get_pod_logs", arguments{"name": "web-0"}); !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("error = %v", err)
	}
	if err := refuses(t, server, "get_pod_logs", arguments{"namespace": "prod"}); !strings.Contains(err.Error(), "name") {
		t.Fatalf("error = %v", err)
	}
}

func TestKeepingOnlyTheLinesThatLookWrong(t *testing.T) {
	cluster := &fakeCluster{lines: []string{"listening", "ERROR could not connect", "ready"}}
	server := serverFor(cluster, Options{})

	lines := as[[]string](t, run(t, server, "get_pod_logs", arguments{
		"namespace": "prod", "name": "web-0", "errorsOnly": true,
	})["lines"])

	if len(lines) != 1 || !strings.Contains(lines[0], "could not connect") {
		t.Fatalf("lines = %v", lines)
	}
}

func TestWhenNothingLooksWrongEveryLineComesBack(t *testing.T) {
	cluster := &fakeCluster{lines: []string{"listening", "ready"}}
	server := serverFor(cluster, Options{})

	lines := as[[]string](t, run(t, server, "get_pod_logs", arguments{
		"namespace": "prod", "name": "web-0", "errorsOnly": true,
	})["lines"])

	if len(lines) != 2 {
		t.Fatalf("lines = %v, want everything rather than nothing", lines)
	}
}

func TestWorkloadLogsFollowThePodSelector(t *testing.T) {
	cluster := &fakeCluster{
		catalog:  catalogOf(descriptor("apps", "v1", "deployments", "Deployment")),
		selector: "app=web",
		lines:    []string{"one", "two"},
	}
	server := serverFor(cluster, Options{})

	result := run(t, server, "get_workload_logs", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod",
	})

	if result["selector"] != "app=web" {
		t.Fatalf("selector = %v", result["selector"])
	}
	if cluster.lastLogs.Selector != "app=web" {
		t.Fatalf("request = %+v, want the selector passed down", cluster.lastLogs)
	}
}

func TestAWorkloadThatSelectsNoPodsSaysSo(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("apps", "v1", "deployments", "Deployment")),
	}
	server := serverFor(cluster, Options{})

	err := refuses(t, server, "get_workload_logs", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod",
	})

	if !strings.Contains(err.Error(), "selects no pods") {
		t.Fatalf("error = %v", err)
	}
}

func TestWorkloadLogsNeedANamespace(t *testing.T) {
	cluster := &fakeCluster{catalog: catalogOf(descriptor("apps", "v1", "deployments", "Deployment"))}
	server := serverFor(cluster, Options{})

	if err := refuses(t, server, "get_workload_logs", arguments{"resource": "deployments", "name": "web"}); err == nil {
		t.Fatal("a workload log read without a namespace was accepted")
	}
}

func TestLogFailuresComeBack(t *testing.T) {
	server := serverFor(&fakeCluster{linesErr: errRefused}, Options{})

	if err := refuses(t, server, "get_pod_logs", arguments{"namespace": "prod", "name": "web-0"}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
	withSelector := serverFor(&fakeCluster{
		catalog:  catalogOf(descriptor("apps", "v1", "deployments", "Deployment")),
		selector: "app=web",
		selErr:   nil,
	}, Options{})
	as[*fakeCluster](t, withSelector.cluster).linesErr = errRefused
	if err := refuses(t, withSelector, "get_workload_logs", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod",
	}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
	broken := serverFor(&fakeCluster{
		catalog: catalogOf(descriptor("apps", "v1", "deployments", "Deployment")),
		selErr:  errRefused,
	}, Options{})
	if err := refuses(t, broken, "get_workload_logs", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod",
	}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
}

// usage

func TestTopRanksByWhatWasAsked(t *testing.T) {
	cluster := &fakeCluster{usage: api.Metrics{Pods: map[string]api.ResourceUsage{
		"prod/web-0":    {CPUMilli: 10, MemoryMi: 900},
		"prod/web-1":    {CPUMilli: 90, MemoryMi: 100},
		"staging/api-0": {CPUMilli: 50, MemoryMi: 500},
	}}}
	server := serverFor(cluster, Options{})

	byCPU := run(t, server, "top_resources", arguments{})
	if byCPU["by"] != "cpu" {
		t.Fatalf("by = %v", byCPU["by"])
	}
	byMemory := run(t, server, "top_resources", arguments{"by": "memory", "limit": float64(1)})
	if byMemory["by"] != "memory" {
		t.Fatalf("by = %v", byMemory["by"])
	}
}

func TestTopKeepsToOneNamespaceWhenAsked(t *testing.T) {
	cluster := &fakeCluster{usage: api.Metrics{Pods: map[string]api.ResourceUsage{
		"prod/web-0":    {CPUMilli: 10},
		"staging/api-0": {CPUMilli: 50},
	}}}
	server := serverFor(cluster, Options{})

	result := run(t, server, "top_resources", arguments{"namespace": "prod"})

	body := result["pods"]
	if strings.Contains(strings.ToLower(stringify(body)), "staging") {
		t.Fatalf("pods = %v, want only the namespace asked for", body)
	}
}

func stringify(value any) string {
	return fmt.Sprint(value)
}

// search and helm

func TestSearchPassesTheQueryThrough(t *testing.T) {
	cluster := &fakeCluster{hits: api.SearchResults{
		Hits:      []api.SearchHit{{Name: "web"}},
		Truncated: true,
	}}
	server := serverFor(cluster, Options{})

	result := run(t, server, "search", arguments{"query": "web"})

	if truncated := as[bool](t, result["truncated"]); !truncated {
		t.Fatalf("truncated = %v, want the cap reported", result["truncated"])
	}
	if err := refuses(t, server, "search", arguments{}); !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestHelmReleasesAndOneRelease(t *testing.T) {
	cluster := &fakeCluster{
		releases: api.HelmReleases{Releases: []api.HelmRelease{{Name: "podinfo"}}},
		release: api.HelmReleaseDetail{
			Release: api.HelmRelease{Name: "podinfo"},
			Values:  "auth:\n  token: not-a-real-value\n",
		},
	}
	server := serverFor(cluster, Options{})

	listed := run(t, server, "list_helm_releases", arguments{})
	if len(as[[]api.HelmRelease](t, listed["releases"])) != 1 {
		t.Fatalf("releases = %v", listed["releases"])
	}
	one := run(t, server, "get_helm_release", arguments{"namespace": "demo", "name": "podinfo"})
	if strings.Contains(as[string](t, one["values"]), "not-a-real-value") {
		t.Fatalf("release values leaked a token: %q", one["values"])
	}
}

func TestHelmFailuresComeBack(t *testing.T) {
	server := serverFor(&fakeCluster{relErr: errRefused, oneRelErr: errRefused}, Options{})

	if err := refuses(t, server, "list_helm_releases", arguments{}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
	if err := refuses(t, server, "get_helm_release", arguments{"namespace": "demo", "name": "x"}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
	if err := refuses(t, server, "get_helm_release", arguments{"name": "x"}); err == nil {
		t.Fatal("a release read with no namespace was accepted")
	}
	if err := refuses(t, server, "get_helm_release", arguments{"namespace": "demo"}); err == nil {
		t.Fatal("a release read with no name was accepted")
	}
}

// the audit

func TestTheAuditNamesTheObjectEachFindingIsAbout(t *testing.T) {
	cluster := &fakeCluster{report: api.CheckReport{
		Scanned: 4,
		Objects: []api.CheckObject{{Name: "web", Namespace: "prod", Kind: "Deployment"}},
		Groups: []api.CheckGroup{{
			ID:       "image-latest",
			Title:    "Image uses a floating tag",
			Severity: "medium",
			Wrong:    "the tag moves",
			Remedy:   "pin a digest",
			Findings: []api.CheckFinding{{Ref: 0, Detail: "web uses :latest"}},
		}},
	}}
	server := serverFor(cluster, Options{})

	rows := as[[]map[string]any](t, run(t, server, "get_cluster_audit", arguments{})["findings"])

	if len(rows) != 1 {
		t.Fatalf("findings = %v", rows)
	}
	found := as[api.CheckObject](t, rows[0]["object"])
	if found.Name != "web" {
		t.Fatalf("object = %+v", found)
	}
	if rows[0]["remedy"] != "pin a digest" {
		t.Fatalf("remedy = %v, want the fix carried with the finding", rows[0]["remedy"])
	}
}

func TestAFindingPointingNowhereCarriesNoObject(t *testing.T) {
	cluster := &fakeCluster{report: api.CheckReport{
		Groups: []api.CheckGroup{{ID: "x", Findings: []api.CheckFinding{{Ref: 9}}}},
	}}
	server := serverFor(cluster, Options{})

	rows := as[[]map[string]any](t, run(t, server, "get_cluster_audit", arguments{})["findings"])

	if rows[0]["object"] != nil {
		t.Fatalf("object = %v, want nothing for a reference out of range", rows[0]["object"])
	}
}

func TestTheAuditFiltersBySeverityAndCheck(t *testing.T) {
	cluster := &fakeCluster{report: api.CheckReport{Groups: []api.CheckGroup{
		{ID: "one", Severity: "high", Findings: []api.CheckFinding{{Detail: "a"}}},
		{ID: "two", Severity: "low", Findings: []api.CheckFinding{{Detail: "b"}}},
	}}}
	server := serverFor(cluster, Options{})

	high := as[[]map[string]any](t, run(t, server, "get_cluster_audit", arguments{"severity": "high"})["findings"])
	if len(high) != 1 || high[0]["check"] != "one" {
		t.Fatalf("findings = %v", high)
	}
	named := as[[]map[string]any](t, run(t, server, "get_cluster_audit", arguments{"check": "two"})["findings"])
	if len(named) != 1 || named[0]["check"] != "two" {
		t.Fatalf("findings = %v", named)
	}
	capped := as[[]map[string]any](t, run(t, server, "get_cluster_audit", arguments{"limit": float64(1)})["findings"])
	if len(capped) != 1 {
		t.Fatalf("findings = %v", capped)
	}
}

// issues

func TestTheIssueQueueScrubsAndCaps(t *testing.T) {
	cluster := &fakeCluster{queue: api.IssueQueue{Rows: []api.Issue{
		{Title: "CrashLoopBackOff", Detail: "exited after password=hunter2000", Folded: 3},
		{Title: "NotProgressing"},
	}}}
	server := serverFor(cluster, Options{})

	rows := as[[]map[string]any](t, run(t, server, "get_issues", arguments{"limit": float64(1)})["rows"])

	if len(rows) != 1 {
		t.Fatalf("rows = %v", rows)
	}
	if strings.Contains(as[string](t, rows[0]["detail"]), "hunter2000") {
		t.Fatalf("an issue detail leaked a secret: %q", rows[0]["detail"])
	}
	if rows[0]["folded"] != 3 {
		t.Fatalf("folded = %v", rows[0]["folded"])
	}
}

// prometheus

func TestPrometheusIsOfferedOnlyWhenThereIsOne(t *testing.T) {
	without := serverFor(&fakeCluster{}, Options{})
	if _, held := without.tools["query_prometheus"]; held {
		t.Fatal("the query tool is offered with no Prometheus behind it")
	}
	with := serverFor(&fakeCluster{}, Options{Prometheus: &fakeProm{}})
	if _, held := with.tools["query_prometheus"]; !held {
		t.Fatal("the query tool is missing when Prometheus is configured")
	}
}

func TestAPromQueryIsPassedThroughAndItsFailureReturned(t *testing.T) {
	source := &fakeProm{samples: []prom.Sample{{Value: 1}}}
	server := serverFor(&fakeCluster{}, Options{Prometheus: source})

	result := run(t, server, "query_prometheus", arguments{"query": "up"})

	if source.asked != "up" {
		t.Fatalf("asked = %q", source.asked)
	}
	if result["query"] != "up" {
		t.Fatalf("query = %v", result["query"])
	}
	if err := refuses(t, server, "query_prometheus", arguments{}); err == nil {
		t.Fatal("a query with no expression was accepted")
	}
	source.err = errRefused
	if err := refuses(t, server, "query_prometheus", arguments{"query": "up"}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
}

func TestTheDashboardShowsOnlyTheFirstFewIssues(t *testing.T) {
	rows := make([]api.Issue, 0, 20)
	for index := range 20 {
		rows = append(rows, api.Issue{Title: "row", Severity: "warning", Object: api.ObjectRef{Name: string(rune('a' + index))}})
	}
	server := serverFor(&fakeCluster{queue: api.IssueQueue{Rows: rows}}, Options{})

	result := run(t, server, "get_dashboard", arguments{})

	if len(as[[]string](t, result["issues"])) != 10 {
		t.Fatalf("issues = %d, want the dashboard to show a handful", len(as[[]string](t, result["issues"])))
	}
	if result["issueCount"] != 20 {
		t.Fatalf("issueCount = %v, want the real total alongside", result["issueCount"])
	}
}

func TestListingSomethingTheClusterHasNoneOf(t *testing.T) {
	cluster := &fakeCluster{catalog: catalogOf(descriptor("", "v1", "pods", "Pod"))}
	server := serverFor(cluster, Options{})

	result := run(t, server, "list_resources", arguments{"resource": "pods"})

	if result["total"] != 0 || len(as[[]map[string]any](t, result["items"])) != 0 {
		t.Fatalf("result = %v, want an empty list rather than nothing", result)
	}
}

func TestAnObjectWithNoCreationStampCarriesNoAge(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod"}}
	item.SetName("web-0")
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("", "v1", "pods", "Pod")),
		listed:  []*unstructured.Unstructured{item},
	}
	server := serverFor(cluster, Options{})

	rows := as[[]map[string]any](t, run(t, server, "list_resources", arguments{"resource": "pods"})["items"])

	if _, aged := rows[0]["age"]; aged {
		t.Fatalf("row = %v, want no age when nothing stamped it", rows[0])
	}
}

func TestTheResourceVersionComesBackSoAnApplyCanCarryIt(t *testing.T) {
	cases := []struct {
		name     string
		document string
		want     string
	}{
		{
			name:     "a quoted version",
			document: "metadata:\n  name: web\n  resourceVersion: \"4021\"\n",
			want:     "4021",
		},
		{
			name:     "an unquoted version",
			document: "metadata:\n  resourceVersion: 77\n",
			want:     "77",
		},
		{
			name:     "a document that states none",
			document: "metadata:\n  name: web\n",
			want:     "",
		},
		{
			name:     "no document at all",
			document: "",
			want:     "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceVersionOf(tc.document); got != tc.want {
				t.Fatalf("resourceVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGettingAResourceCarriesItsVersionForTheRoundTrip(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("", "v1", "configmaps", "ConfigMap")),
		detail: api.ObjectDetail{
			Kind: "ConfigMap",
			Name: "settings",
			YAML: "metadata:\n  name: settings\n  resourceVersion: \"9\"\n",
		},
	}
	server := serverFor(cluster, Options{})

	result := run(t, server, "get_resource", arguments{argResource: "configmaps", argName: "settings"})

	if result["resourceVersion"] != "9" {
		t.Fatalf("resourceVersion = %v, want the one an apply must carry back", result["resourceVersion"])
	}
}

func TestASecretStillGivesUpNoVersionBecauseItGivesUpNoDocument(t *testing.T) {
	cluster := &fakeCluster{
		catalog: catalogOf(descriptor("", "v1", "secrets", "Secret")),
		detail: api.ObjectDetail{
			Kind: "Secret",
			Name: "tls",
			YAML: "metadata:\n  resourceVersion: \"9\"\ndata:\n  password: aHVudGVyMg==\n",
		},
	}
	server := serverFor(cluster, Options{})

	result := run(t, server, "get_resource", arguments{argResource: "secrets", argName: "tls"})

	if result["yaml"] != "" {
		t.Fatalf("a Secret gave up its document: %q", result["yaml"])
	}
	if result["resourceVersion"] != "9" {
		t.Fatalf("resourceVersion = %v; it is not secret and an apply needs it", result["resourceVersion"])
	}
}
