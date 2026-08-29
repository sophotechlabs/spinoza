package mcp

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/topology"
)

const (
	defaultRows    = 100
	defaultTop     = 20
	defaultTailFor = 200
)

func (s *Server) registerReads() {
	s.register(tool{
		name:        "get_dashboard",
		title:       "Cluster dashboard",
		description: "Cluster health in one call: versions, node and pod counts, what is failing now, and how many best-practice findings stand.",
		run:         s.dashboard,
	})
	s.register(tool{
		name:        "list_namespaces",
		title:       "Namespaces",
		description: "Every namespace this cluster reports, with its phase.",
		run:         s.namespaces,
	})
	s.register(tool{
		name:        "list_resource_kinds",
		title:       "Resource kinds",
		description: "Every resource type the cluster serves, grouped as spinoza groups them. Ask this before guessing a resource name.",
		run:         s.kinds,
	})
	s.register(tool{
		name:        "list_resources",
		title:       "List resources",
		description: "Objects of one kind, summarized to name, namespace, age and readiness.",
		properties: map[string]propOf{
			argResource:  text("Resource name or kind, for example deployments or Deployment."),
			argGroup:     text("API group, when two groups serve the same name."),
			argNamespace: text("Namespace to limit to. Omit for every namespace."),
			argLimit:     number("How many to return. Defaults to 100."),
		},
		required: []string{argResource},
		run:      s.listResources,
	})
	s.register(tool{
		name:        "get_resource",
		title:       "Get one resource",
		description: "One object with its conditions, containers, owners and optionally its events. Secret values are never returned.",
		properties: map[string]propOf{
			argResource:  text("Resource name or kind."),
			argName:      text("Object name."),
			argNamespace: text("Namespace, for a namespaced kind."),
			argGroup:     text("API group, when ambiguous."),
			"events":     toggle("Include the object's events."),
		},
		required: []string{argResource, argName},
		run:      s.getResource,
	})
	s.register(tool{
		name:        "get_events",
		title:       "Events",
		description: "Events deduplicated by reason and message, newest first.",
		properties: map[string]propOf{
			argNamespace: text("Namespace to limit to."),
			"uid":        text("Object uid, to scope to one object."),
			argLimit:     number("How many to return. Defaults to 100."),
		},
		run: s.events,
	})
	s.register(tool{
		name:        "get_topology",
		title:       "Ownership topology",
		description: "The ownership and wiring graph as nodes and edges, folded so one Deployment is one node.",
		properties: map[string]propOf{
			argNamespace: text("Namespace to scope to. Omit for the whole cluster."),
			"expand":     text("Comma-separated node ids to open one level."),
		},
		run: s.topology,
	})
	s.register(tool{
		name:        "get_pod_logs",
		title:       "Pod logs",
		description: "Logs from one pod, secret-redacted, with errors and warnings kept first.",
		properties: map[string]propOf{
			argNamespace: text("Pod namespace."),
			argName:      text("Pod name."),
			"container":  text("Container name. Omit for the first."),
			"tail":       number("Lines to read. Defaults to 200."),
			"errorsOnly": toggle("Keep only lines that look like errors or warnings."),
		},
		required: []string{argNamespace, argName},
		run:      s.podLogs,
	})
	s.register(tool{
		name:        "get_workload_logs",
		title:       "Workload logs",
		description: "Logs from every pod of a workload at once, secret-redacted and deduplicated.",
		properties: map[string]propOf{
			argResource:  text("Workload resource or kind, for example deployments."),
			argName:      text("Workload name."),
			argNamespace: text("Namespace."),
			argGroup:     text("API group, when ambiguous."),
			"tail":       number("Lines per pod. Defaults to 200."),
			"errorsOnly": toggle("Keep only lines that look like errors or warnings."),
		},
		required: []string{argResource, argName, argNamespace},
		run:      s.workloadLogs,
	})
	s.register(tool{
		name:        "top_resources",
		title:       "Top consumers",
		description: "Pods ranked by live CPU or memory.",
		properties: map[string]propOf{
			"by":         choice("What to rank by. Defaults to cpu.", "cpu", "memory"),
			argNamespace: text("Namespace to limit to."),
			argLimit:     number("How many to return. Defaults to 20."),
		},
		run: s.top,
	})
	s.register(tool{
		name:        "search",
		title:       "Search the cluster",
		description: "Find objects by name across every kind spinoza has cached.",
		properties: map[string]propOf{
			argQuery: text("What to look for."),
		},
		required: []string{argQuery},
		run:      s.search,
	})
	s.register(tool{
		name:        "list_helm_releases",
		title:       "Helm releases",
		description: "Every Helm release, with chart, version and status.",
		run:         s.helmReleases,
	})
	s.register(tool{
		name:        "get_helm_release",
		title:       "Get a Helm release",
		description: "One release with its history and the values it was installed with.",
		properties: map[string]propOf{
			argNamespace: text("Release namespace."),
			argName:      text("Release name."),
		},
		required: []string{argNamespace, argName},
		run:      s.helmRelease,
	})
	s.register(tool{
		name:        "get_cluster_audit",
		title:       "Best-practice audit",
		description: "Findings from spinoza's checks, with the object each one names and what to change.",
		properties: map[string]propOf{
			"severity": choice("Keep only this severity.", "high", "medium", "low"),
			"check":    text("Keep only this check id."),
			argLimit:   number("How many findings to return. Defaults to 100."),
		},
		run: s.audit,
	})
	s.register(tool{
		name:        "get_issues",
		title:       "What is broken now",
		description: "The live queue of what is failing, root cause first, with pods folded under the workload that owns them.",
		properties: map[string]propOf{
			argLimit: number("How many rows. Defaults to 100."),
		},
		run: s.issues,
	})
	if s.prom != nil {
		s.register(tool{
			name:        "query_prometheus",
			title:       "Query Prometheus",
			description: "Run an instant PromQL query against the Prometheus spinoza discovered in this cluster.",
			properties: map[string]propOf{
				argQuery: text("PromQL expression."),
			},
			required: []string{argQuery},
			run:      s.queryProm,
		})
	}
}

func (s *Server) dashboard(ctx context.Context, _ arguments) (any, error) {
	overview := s.cluster.Overview(ctx)
	counts := s.cluster.Counts(ctx)
	queue := s.cluster.Issues(ctx)
	report := s.cluster.Checks(ctx)
	return map[string]any{
		"context":     s.context,
		"kubernetes":  overview.Version,
		"nodes":       overview.Nodes,
		"pods":        overview.Pods,
		"warnings":    overview.Warnings,
		"failing":     failingSummary(counts),
		"issues":      issueLines(queue, 10),
		"issueCount":  len(queue.Rows),
		"auditGroups": len(report.Groups),
		"auditFound":  findingCount(report),
		"errors":      trimErrors(overview.Error, queue.Error, report.Error),
	}, nil
}

func (s *Server) namespaces(ctx context.Context, _ arguments) (any, error) {
	found := s.cluster.Namespaces(ctx)
	out := make([]map[string]string, 0, len(found.Names))
	for _, name := range found.Names {
		out = append(out, map[string]string{argName: name})
	}
	return map[string]any{"namespaces": out, keyError: found.Error}, nil
}

func (s *Server) kinds(_ context.Context, _ arguments) (any, error) {
	catalog := s.cluster.Resources()
	out := map[string][]string{}
	for _, category := range catalog.Categories {
		for _, desc := range category.Resources {
			out[category.Name] = append(out[category.Name], label(desc))
		}
	}
	return map[string]any{"kinds": out, keyError: catalog.Error}, nil
}

func label(desc api.ResourceDescriptor) string {
	if desc.Group == "" {
		return desc.Resource
	}
	return desc.Resource + "." + desc.Group
}

func (s *Server) listResources(ctx context.Context, args arguments) (any, error) {
	ref, err := args.kind(s.cluster.Resources())
	if err != nil {
		return nil, err
	}
	items, err := s.cluster.ListKind(ctx, ref)
	if err != nil {
		return nil, err
	}
	limit := args.number(argLimit, defaultRows)
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if ref.Namespace != "" && item.GetNamespace() != ref.Namespace {
			continue
		}
		if len(rows) == limit {
			break
		}
		rows = append(rows, summaryOf(item))
	}
	return map[string]any{argResource: ref.Resource, "total": len(items), "returned": len(rows), "items": rows}, nil
}

func summaryOf(item *unstructured.Unstructured) map[string]any {
	out := map[string]any{argName: item.GetName()}
	if item.GetNamespace() != "" {
		out["namespace"] = item.GetNamespace()
	}
	if created := item.GetCreationTimestamp(); !created.IsZero() {
		out["age"] = time.Since(created.Time).Truncate(time.Second).String()
	}
	return out
}

func (s *Server) getResource(ctx context.Context, args arguments) (any, error) {
	ref, err := args.ref(s.cluster.Resources())
	if err != nil {
		return nil, err
	}
	detail, err := s.cluster.Object(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"kind":            detail.Kind,
		"resourceVersion": resourceVersionOf(detail.YAML),
		argName:           detail.Name,
		argNamespace:      detail.Namespace,
		"conditions":      detail.Conditions,
		"containers":      detail.Containers,
		"owners":          detail.Owners,
		"labels":          scrubMap(detail.Labels),
		"replicas":        detail.Replicas,
		"managedBy":       detail.ManagedBy,
		argYAML:           safeYAML(detail.Kind, detail.YAML),
	}
	if len(detail.Data) > 0 {
		out["dataKeys"] = keysOnly(detail.Data)
	}
	if carriesSecrets(detail.Kind) {
		out["note"] = "spinoza withholds Secret values and the raw document; key names and sizes are above"
	}
	if args.flag("events") {
		events, eventsErr := s.cluster.Events(ctx, detail.Namespace, detail.UID)
		if eventsErr != nil {
			return nil, eventsErr
		}
		out["events"] = dedupeEvents(events, defaultRows)
	}
	return out, nil
}

var versionLine = regexp.MustCompile(`(?m)^\s*resourceVersion:\s*"?(\d+)"?\s*$`)

func resourceVersionOf(document string) string {
	found := versionLine.FindStringSubmatch(document)
	if found == nil {
		return ""
	}
	return found[1]
}

func (s *Server) events(ctx context.Context, args arguments) (any, error) {
	found, err := s.cluster.Events(ctx, args.text(argNamespace), args.text("uid"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"events": dedupeEvents(found, args.number(argLimit, defaultRows))}, nil
}

func dedupeEvents(found []api.Event, limit int) []map[string]any {
	seen := map[string]int{}
	out := []map[string]any{}
	for _, event := range found {
		key := event.Reason + "|" + event.Message
		at, held := seen[key]
		if held {
			running, ok := out[at]["count"].(int)
			if ok {
				out[at]["count"] = running + int(event.Count)
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, map[string]any{
			"type":     event.Type,
			"reason":   event.Reason,
			"message":  scrub(event.Message),
			"source":   event.Source,
			"lastSeen": event.LastSeen,
			"count":    int(event.Count),
		})
	}
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

func (s *Server) topology(ctx context.Context, args arguments) (any, error) {
	req := topology.Request{Namespace: args.text(argNamespace)}
	if open := args.text("expand"); open != "" {
		req.Expanded = strings.Split(open, ",")
	}
	graph := s.cluster.Topology(ctx, req)
	nodes := make([]map[string]any, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		row := map[string]any{
			"id":         node.ID,
			"kind":       node.Kind,
			argName:      node.Name,
			argNamespace: node.Namespace,
			"ready":      node.Ready,
			"status":     node.Status,
		}
		if node.Contains > 0 {
			row["folded"] = node.Contains
			row["foldedUnhealthy"] = node.Unhealthy
		}
		nodes = append(nodes, row)
	}
	edges := make([]map[string]string, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		edges = append(edges, map[string]string{"from": edge.From, "to": edge.To, "kind": edge.Kind})
	}
	return map[string]any{"nodes": nodes, "edges": edges, keyError: graph.Error}, nil
}

func (s *Server) podLogs(ctx context.Context, args arguments) (any, error) {
	namespace, err := args.required(argNamespace)
	if err != nil {
		return nil, err
	}
	name, err := args.required(argName)
	if err != nil {
		return nil, err
	}
	lines, err := s.readLogs(ctx, logs.Request{
		Namespace: namespace,
		Name:      name,
		Container: args.text("container"),
		TailLines: int64(args.number("tail", defaultTailFor)),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"pod": name, "lines": pick(lines, args.flag("errorsOnly"))}, nil
}

func (s *Server) workloadLogs(ctx context.Context, args arguments) (any, error) {
	ref, err := args.refIn(s.cluster.Resources())
	if err != nil {
		return nil, err
	}
	selector, err := s.cluster.PodSelector(ctx, ref)
	if err != nil {
		return nil, err
	}
	if selector == "" {
		return nil, fmt.Errorf("%s/%s selects no pods", ref.Resource, ref.Name)
	}
	lines, err := s.readLogs(ctx, logs.Request{
		Namespace: ref.Namespace,
		Selector:  selector,
		TailLines: int64(args.number("tail", defaultTailFor)),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"workload": ref.Name, "selector": selector, "lines": pick(lines, args.flag("errorsOnly"))}, nil
}

func (s *Server) readLogs(ctx context.Context, req logs.Request) ([]string, error) {
	if req.TailLines <= 0 {
		req.TailLines = int64(s.logLines)
	}
	req.TailLines = min(req.TailLines, int64(s.logLines))
	collected, err := s.cluster.LogLines(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(collected) > s.logLines {
		collected = collected[:s.logLines]
	}
	return scrubLines(collected), nil
}

var noisy = []string{keyError, "err ", "fatal", "panic", "warn", "exception", "failed", "failure"}

func pick(lines []string, errorsOnly bool) []string {
	if !errorsOnly {
		return lines
	}
	kept := []string{}
	for _, line := range lines {
		lowered := strings.ToLower(line)
		for _, mark := range noisy {
			if strings.Contains(lowered, mark) {
				kept = append(kept, line)
				break
			}
		}
	}
	if len(kept) == 0 {
		return lines
	}
	return kept
}

func (s *Server) top(ctx context.Context, args arguments) (any, error) {
	usage := s.cluster.Metrics(ctx)
	by := args.text("by")
	namespace := args.text(argNamespace)
	type row struct {
		Pod    string `json:"pod"`
		CPU    int64  `json:"cpuMilli"`
		Memory int64  `json:"memoryMi"`
	}
	rows := []row{}
	for pod, used := range usage.Pods {
		if namespace != "" && !strings.HasPrefix(pod, namespace+"/") {
			continue
		}
		rows = append(rows, row{Pod: pod, CPU: used.CPUMilli, Memory: used.MemoryMi})
	}
	slices.SortFunc(rows, func(left, right row) int {
		if by == "memory" {
			return cmp.Compare(right.Memory, left.Memory)
		}
		return cmp.Compare(right.CPU, left.CPU)
	})
	limit := args.number(argLimit, defaultTop)
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return map[string]any{"by": rankedBy(by), "pods": rows, keyError: usage.Error}, nil
}

func rankedBy(by string) string {
	if by == "memory" {
		return "memory"
	}
	return "cpu"
}

func (s *Server) search(ctx context.Context, args arguments) (any, error) {
	query, err := args.required(argQuery)
	if err != nil {
		return nil, err
	}
	found := s.cluster.Search(ctx, query)
	return map[string]any{"hits": found.Hits, "truncated": found.Truncated, "errors": found.Errors}, nil
}

func (s *Server) helmReleases(ctx context.Context, _ arguments) (any, error) {
	found, err := s.cluster.HelmReleases(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"releases": found.Releases, keyError: found.Error}, nil
}

func (s *Server) helmRelease(ctx context.Context, args arguments) (any, error) {
	namespace, err := args.required(argNamespace)
	if err != nil {
		return nil, err
	}
	name, err := args.required(argName)
	if err != nil {
		return nil, err
	}
	detail, err := s.cluster.HelmRelease(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"release": detail.Release,
		"history": detail.History,
		"values":  scrub(detail.Values),
		keyError:  detail.Error,
	}, nil
}

func (s *Server) audit(ctx context.Context, args arguments) (any, error) {
	report := s.cluster.Checks(ctx)
	severity := args.text("severity")
	only := args.text("check")
	limit := args.number(argLimit, defaultRows)
	rows := []map[string]any{}
	for _, group := range report.Groups {
		if only != "" && group.ID != only {
			continue
		}
		if severity != "" && group.Severity != severity {
			continue
		}
		for _, finding := range group.Findings {
			if len(rows) == limit {
				break
			}
			rows = append(rows, map[string]any{
				"check":    group.ID,
				"title":    group.Title,
				"severity": group.Severity,
				"wrong":    group.Wrong,
				"remedy":   group.Remedy,
				"object":   objectAt(report, finding.Ref),
				"detail":   finding.Detail,
			})
		}
	}
	return map[string]any{"scanned": report.Scanned, "findings": rows, keyError: report.Error}, nil
}

func objectAt(report api.CheckReport, ref int) any {
	if ref < 0 || ref >= len(report.Objects) {
		return nil
	}
	return report.Objects[ref]
}

func (s *Server) issues(ctx context.Context, args arguments) (any, error) {
	queue := s.cluster.Issues(ctx)
	return map[string]any{"rows": issueRows(queue, args.number(argLimit, defaultRows)), keyError: queue.Error}, nil
}

func issueRows(queue api.IssueQueue, limit int) []map[string]any {
	out := []map[string]any{}
	for _, row := range queue.Rows {
		if len(out) == limit {
			break
		}
		out = append(out, map[string]any{
			"title":    row.Title,
			"severity": row.Severity,
			"kind":     row.Kind,
			"object":   row.Object,
			"detail":   scrub(row.Detail),
			argAction:  row.Action,
			"folded":   row.Folded,
		})
	}
	return out
}

func issueLines(queue api.IssueQueue, limit int) []string {
	out := []string{}
	for _, row := range queue.Rows {
		if len(out) == limit {
			break
		}
		out = append(out, row.Severity+": "+row.Title+" — "+row.Object.Name)
	}
	return out
}

func failingSummary(counts api.ResourceCounts) map[string]int {
	out := make(map[string]int, len(counts.Failing))
	maps.Copy(out, counts.Failing)
	return out
}

func findingCount(report api.CheckReport) int {
	total := 0
	for _, group := range report.Groups {
		total += len(group.Findings)
	}
	return total
}

func trimErrors(reported ...string) []string {
	out := []string{}
	for _, one := range reported {
		if one == "" {
			continue
		}
		if slices.Contains(out, one) {
			continue
		}
		out = append(out, one)
	}
	return out
}

type series struct {
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
}

func seriesOf(samples []prom.Sample) []series {
	out := make([]series, 0, len(samples))
	for _, one := range samples {
		out = append(out, series{Labels: one.Labels, Value: one.Value})
	}
	return out
}

func (s *Server) queryProm(ctx context.Context, args arguments) (any, error) {
	query, err := args.required(argQuery)
	if err != nil {
		return nil, err
	}
	samples, err := s.prom.Instant(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{argQuery: query, "samples": seriesOf(samples)}, nil
}
