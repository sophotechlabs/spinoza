package resources

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/unstr"
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
	case "Event":
		return []api.Column{
			{Name: "Type", Render: "status"},
			{Name: "Reason"},
			{Name: "Object"},
			{Name: "Count"},
			{Name: "Last seen", Render: "age"},
			{Name: "Message"},
		}
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
		return []string{unstr.String(obj, "status", "phase")}
	case "Job":
		return jobCells(obj)
	case "Event":
		return eventCells(obj)
	default:
		return []string{unstr.ReadySummary(obj)}
	}
}

func eventCells(obj *unstructured.Unstructured) []string {
	return []string{
		unstr.String(obj, "type"),
		unstr.String(obj, "reason"),
		eventObject(obj),
		eventCount(obj),
		eventLastSeen(obj),
		eventMessage(obj),
	}
}

func eventObject(obj *unstructured.Unstructured) string {
	for _, field := range []string{"involvedObject", "regarding"} {
		kind := unstr.String(obj, field, "kind")
		name := unstr.String(obj, field, "name")
		if kind == "" && name == "" {
			continue
		}
		if kind == "" {
			return name
		}
		return kind + "/" + name
	}
	return ""
}

func eventMessage(obj *unstructured.Unstructured) string {
	message := unstr.String(obj, "message")
	if message != "" {
		return message
	}
	return unstr.String(obj, "note")
}

func eventCount(obj *unstructured.Unstructured) string {
	count := toInt64(obj.Object["count"])
	if count == 0 {
		count = toInt64(obj.Object["deprecatedCount"])
	}
	if count == 0 {
		series, ok := unstr.Map(obj, "series")
		if ok {
			count = toInt64(series["count"])
		}
	}
	if count == 0 {
		return ""
	}
	return strconv.FormatInt(count, 10)
}

func eventLastSeen(obj *unstructured.Unstructured) string {
	for _, path := range [][]string{
		{"lastTimestamp"},
		{"series", "lastObservedTime"},
		{"deprecatedLastTimestamp"},
		{"eventTime"},
		{"firstTimestamp"},
	} {
		found := unstr.String(obj, path...)
		if found != "" {
			return found
		}
	}
	return ""
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
	for _, s := range unstr.Slice(u, "status", field) {
		entry, ok := s.(map[string]any)
		if !ok {
			continue
		}
		name := unstr.At(entry, "name")
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
		termReason := unstr.At(term, "reason")
		return "terminated", termReason
	}
	if wait, ok := stateFields["waiting"].(map[string]any); ok {
		waitReason := unstr.At(wait, "reason")
		return "waiting", waitReason
	}
	return "waiting", ""
}

func podCells(obj *unstructured.Unstructured) []string {
	total := len(unstr.Slice(obj, "spec", "containers"))
	ready := 0
	var restarts int64
	for _, s := range unstr.Slice(obj, "status", "containerStatuses") {
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
		unstr.String(obj, "status", "phase"),
		strconv.FormatInt(restarts, 10),
		unstr.String(obj, "spec", "nodeName"),
	}
}

func workloadCells(u *unstructured.Unstructured) []string {
	desired := unstr.Int(u, "spec", "replicas")
	ready := unstr.Int(u, "status", "readyReplicas")
	updated := unstr.Int(u, "status", "updatedReplicas")
	available := unstr.Int(u, "status", "availableReplicas")
	return []string{
		fmt.Sprintf("%d/%d", ready, desired),
		strconv.FormatInt(updated, 10),
		strconv.FormatInt(available, 10),
	}
}

func daemonCells(u *unstructured.Unstructured) []string {
	return []string{
		strconv.FormatInt(unstr.Int(u, "status", "desiredNumberScheduled"), 10),
		strconv.FormatInt(unstr.Int(u, "status", "numberReady"), 10),
		strconv.FormatInt(unstr.Int(u, "status", "numberAvailable"), 10),
	}
}

func serviceCells(obj *unstructured.Unstructured) []string {
	parts := []string{}
	for _, p := range unstr.Slice(obj, "spec", "ports") {
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
		unstr.String(obj, "spec", "type"),
		unstr.String(obj, "spec", "clusterIP"),
		strings.Join(parts, ","),
	}
}

func nodeCells(obj *unstructured.Unstructured) []string {
	status := "NotReady"
	for _, c := range unstr.Slice(obj, "status", "conditions") {
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
	return []string{status, strings.Join(roles, ","), unstr.String(obj, "status", "nodeInfo", "kubeletVersion")}
}

func jobCells(u *unstructured.Unstructured) []string {
	return []string{fmt.Sprintf("%d/%d", unstr.Int(u, "status", "succeeded"), unstr.Int(u, "spec", "completions"))}
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
