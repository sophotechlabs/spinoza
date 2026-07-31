package resources

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const readyColumn = "Ready"

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
			{Name: readyColumn, Render: "ratio"},
			{Name: "Up-to-date"},
			{Name: "Available"},
		}
	case "DaemonSet":
		return cols("Desired", readyColumn, "Available")
	case "Service":
		return cols("Type", "Cluster-IP", "Ports")
	case "Node":
		return []api.Column{statusColumn(), {Name: "Roles"}, {Name: "Version"}}
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

func cellsFor(obj *unstructured.Unstructured, kind string) []string {
	switch kind {
	case "Pod":
		return podCells(obj)
	case "Deployment", "ReplicaSet", "StatefulSet", "ReplicationController":
		return workloadCells(obj)
	case "DaemonSet":
		return daemonCells(obj)
	case "Service":
		return serviceCells(obj)
	case "Node":
		return nodeCells(obj)
	case "Namespace":
		return []string{nestedString(obj, "status", "phase")}
	case "Job":
		return jobCells(obj)
	default:
		return []string{conditionSummary(obj)}
	}
}

func containersFor(obj *unstructured.Unstructured, kind string) []api.ContainerState {
	if kind != "Pod" {
		return nil
	}
	states := containerStates(obj, "initContainerStatuses", true, false)
	states = append(states, containerStates(obj, "containerStatuses", false, false)...)
	states = append(states, containerStates(obj, "ephemeralContainerStatuses", false, true)...)
	if len(states) == 0 {
		return nil
	}
	return states
}

func containerStates(u *unstructured.Unstructured, field string, init, ephemeral bool) []api.ContainerState {
	out := []api.ContainerState{}
	for _, s := range nestedSlice(u, "status", field) {
		entry, ok := s.(map[string]any)
		if !ok {
			continue
		}
		name := stringAt(entry, "name")
		ready := false
		if b, ok := entry["ready"].(bool); ok {
			ready = b
		}
		state, reason := containerStateReason(entry)
		out = append(out, api.ContainerState{
			Name:      name,
			State:     state,
			Reason:    reason,
			Ready:     ready,
			Restarts:  toInt64(entry["restartCount"]),
			Init:      init,
			Ephemeral: ephemeral,
		})
	}
	return out
}

func containerStateReason(status map[string]any) (state, reason string) {
	stateFields, ok := status["state"].(map[string]any)
	if !ok {
		return "waiting", ""
	}
	if _, ok := stateFields["running"]; ok {
		return "running", ""
	}
	if term, ok := stateFields["terminated"].(map[string]any); ok {
		termReason := stringAt(term, "reason")
		return "terminated", termReason
	}
	if wait, ok := stateFields["waiting"].(map[string]any); ok {
		waitReason := stringAt(wait, "reason")
		return "waiting", waitReason
	}
	return "waiting", ""
}

func podCells(obj *unstructured.Unstructured) []string {
	total := len(nestedSlice(obj, "spec", "containers"))
	ready := 0
	var restarts int64
	for _, s := range nestedSlice(obj, "status", "containerStatuses") {
		entry, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if b, ok := entry["ready"].(bool); ok && b {
			ready++
		}
		restarts += toInt64(entry["restartCount"])
	}
	return []string{
		fmt.Sprintf("%d/%d", ready, total),
		nestedString(obj, "status", "phase"),
		strconv.FormatInt(restarts, 10),
		nestedString(obj, "spec", "nodeName"),
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

func serviceCells(obj *unstructured.Unstructured) []string {
	parts := []string{}
	for _, p := range nestedSlice(obj, "spec", "ports") {
		entry, ok := p.(map[string]any)
		if !ok {
			continue
		}
		proto := ""
		if s, ok := entry["protocol"].(string); ok {
			proto = s
		}
		parts = append(parts, fmt.Sprintf("%d/%s", toInt64(entry["port"]), proto))
	}
	return []string{
		nestedString(obj, "spec", "type"),
		nestedString(obj, "spec", "clusterIP"),
		strings.Join(parts, ","),
	}
}

func nodeCells(obj *unstructured.Unstructured) []string {
	status := "NotReady"
	for _, c := range nestedSlice(obj, "status", "conditions") {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] == "Ready" && m["status"] == "True" {
			status = "Ready"
		}
	}
	unschedulable, _, err := unstructured.NestedBool(obj.Object, "spec", "unschedulable")
	if err == nil && unschedulable {
		status += ",SchedulingDisabled"
	}
	roles := []string{}
	for k := range obj.GetLabels() {
		if !strings.HasPrefix(k, "node-role.kubernetes.io/") {
			continue
		}
		role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
		if role != "" {
			roles = append(roles, role)
		}
	}
	return []string{status, strings.Join(roles, ","), nestedString(obj, "status", "nodeInfo", "kubeletVersion")}
}

func jobCells(u *unstructured.Unstructured) []string {
	return []string{fmt.Sprintf("%d/%d", nestedInt(u, "status", "succeeded"), nestedInt(u, "spec", "completions"))}
}

func conditionSummary(u *unstructured.Unstructured) string {
	for _, c := range nestedSlice(u, "status", "conditions") {
		entry, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if entry["type"] != "Ready" {
			continue
		}
		if entry["status"] == "True" {
			return "Ready"
		}
		if reason, ok := entry["reason"].(string); ok && reason != "" {
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
