package resources

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func columnsFor(kind string) []api.Column {
	switch kind {
	case "Pod":
		return []api.Column{
			{Name: "Containers", Render: "containers"},
			statusColumn(),
			{Name: "Restarts", Render: "restarts"},
			{Name: "Node"},
		}
	case "Deployment", "ReplicaSet", "StatefulSet", "ReplicationController":
		return []api.Column{
			{Name: "Ready", Render: "ratio"},
			{Name: "Up-to-date"},
			{Name: "Available"},
		}
	case "DaemonSet":
		return cols("Desired", "Ready", "Available")
	case "Service":
		return cols("Type", "Cluster-IP", "Ports")
	case "Node":
		return []api.Column{statusColumn(), {Name: "Roles"}, {Name: "Version"}}
	case "Namespace":
		return []api.Column{statusColumn()}
	case "Job":
		return []api.Column{{Name: "Completions", Render: "ratio"}}
	default:
		return []api.Column{statusColumn()}
	}
}

func statusColumn() api.Column {
	return api.Column{Name: "Status", Render: "status"}
}

func cols(names ...string) []api.Column {
	out := make([]api.Column, 0, len(names))
	for _, n := range names {
		out = append(out, api.Column{Name: n})
	}
	return out
}

func cellsFor(u *unstructured.Unstructured, kind string) []string {
	switch kind {
	case "Pod":
		return podCells(u)
	case "Deployment", "ReplicaSet", "StatefulSet", "ReplicationController":
		return workloadCells(u)
	case "DaemonSet":
		return daemonCells(u)
	case "Service":
		return serviceCells(u)
	case "Node":
		return nodeCells(u)
	case "Namespace":
		return []string{nestedString(u, "status", "phase")}
	case "Job":
		return jobCells(u)
	default:
		return []string{conditionSummary(u)}
	}
}

func containersFor(u *unstructured.Unstructured, kind string) []api.ContainerState {
	if kind != "Pod" {
		return nil
	}
	states := containerStates(u, "initContainerStatuses", true)
	states = append(states, containerStates(u, "containerStatuses", false)...)
	if len(states) == 0 {
		return nil
	}
	return states
}

func containerStates(u *unstructured.Unstructured, field string, init bool) []api.ContainerState {
	out := []api.ContainerState{}
	for _, s := range nestedSlice(u, "status", field) {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		name := stringAt(m, "name")
		ready := false
		if b, ok := m["ready"].(bool); ok {
			ready = b
		}
		state, reason := containerStateReason(m)
		out = append(out, api.ContainerState{
			Name:     name,
			State:    state,
			Reason:   reason,
			Ready:    ready,
			Restarts: toInt64(m["restartCount"]),
			Init:     init,
		})
	}
	return out
}

func containerStateReason(m map[string]any) (state, reason string) {
	s, ok := m["state"].(map[string]any)
	if !ok {
		return "waiting", ""
	}
	if _, ok := s["running"]; ok {
		return "running", ""
	}
	if term, ok := s["terminated"].(map[string]any); ok {
		termReason := stringAt(term, "reason")
		return "terminated", termReason
	}
	if wait, ok := s["waiting"].(map[string]any); ok {
		waitReason := stringAt(wait, "reason")
		return "waiting", waitReason
	}
	return "waiting", ""
}

func podCells(u *unstructured.Unstructured) []string {
	total := len(nestedSlice(u, "spec", "containers"))
	ready := 0
	var restarts int64
	for _, s := range nestedSlice(u, "status", "containerStatuses") {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if b, ok := m["ready"].(bool); ok && b {
			ready++
		}
		restarts += toInt64(m["restartCount"])
	}
	return []string{
		fmt.Sprintf("%d/%d", ready, total),
		nestedString(u, "status", "phase"),
		strconv.FormatInt(restarts, 10),
		nestedString(u, "spec", "nodeName"),
	}
}

func workloadCells(u *unstructured.Unstructured) []string {
	desired := nestedInt(u, "spec", "replicas")
	ready := nestedInt(u, "status", "readyReplicas")
	updated := nestedInt(u, "status", "updatedReplicas")
	available := nestedInt(u, "status", "availableReplicas")
	return []string{
		fmt.Sprintf("%d/%d", ready, desired),
		strconv.FormatInt(updated, 10),
		strconv.FormatInt(available, 10),
	}
}

func daemonCells(u *unstructured.Unstructured) []string {
	return []string{
		strconv.FormatInt(nestedInt(u, "status", "desiredNumberScheduled"), 10),
		strconv.FormatInt(nestedInt(u, "status", "numberReady"), 10),
		strconv.FormatInt(nestedInt(u, "status", "numberAvailable"), 10),
	}
}

func serviceCells(u *unstructured.Unstructured) []string {
	parts := []string{}
	for _, p := range nestedSlice(u, "spec", "ports") {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		proto := ""
		if s, ok := m["protocol"].(string); ok {
			proto = s
		}
		parts = append(parts, fmt.Sprintf("%d/%s", toInt64(m["port"]), proto))
	}
	return []string{
		nestedString(u, "spec", "type"),
		nestedString(u, "spec", "clusterIP"),
		strings.Join(parts, ","),
	}
}

func nodeCells(u *unstructured.Unstructured) []string {
	status := "NotReady"
	for _, c := range nestedSlice(u, "status", "conditions") {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] == "Ready" && m["status"] == "True" {
			status = "Ready"
		}
	}
	roles := []string{}
	for k := range u.GetLabels() {
		if !strings.HasPrefix(k, "node-role.kubernetes.io/") {
			continue
		}
		role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
		if role != "" {
			roles = append(roles, role)
		}
	}
	return []string{status, strings.Join(roles, ","), nestedString(u, "status", "nodeInfo", "kubeletVersion")}
}

func jobCells(u *unstructured.Unstructured) []string {
	return []string{fmt.Sprintf("%d/%d", nestedInt(u, "status", "succeeded"), nestedInt(u, "spec", "completions"))}
}

func conditionSummary(u *unstructured.Unstructured) string {
	for _, c := range nestedSlice(u, "status", "conditions") {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] != "Ready" {
			continue
		}
		if m["status"] == "True" {
			return "Ready"
		}
		if reason, ok := m["reason"].(string); ok && reason != "" {
			return reason
		}
		return "NotReady"
	}
	return ""
}

func nestedString(u *unstructured.Unstructured, fields ...string) string {
	v, found, err := unstructured.NestedString(u.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return v
}

func nestedInt(u *unstructured.Unstructured, fields ...string) int64 {
	v, found, err := unstructured.NestedInt64(u.Object, fields...)
	if !found || err != nil {
		return 0
	}
	return v
}

func nestedSlice(u *unstructured.Unstructured, fields ...string) []any {
	v, found, err := unstructured.NestedSlice(u.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return v
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func stringAt(m map[string]any, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}
