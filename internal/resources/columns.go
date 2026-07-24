package resources

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func columnsFor(kind string) []api.Column {
	switch kind {
	case "Pod":
		return cols("Ready", "Status", "Restarts", "Node")
	case "Deployment", "ReplicaSet", "StatefulSet", "ReplicationController":
		return cols("Ready", "Up-to-date", "Available")
	case "DaemonSet":
		return cols("Desired", "Ready", "Available")
	case "Service":
		return cols("Type", "Cluster-IP", "Ports")
	case "Node":
		return cols("Status", "Roles", "Version")
	case "Namespace":
		return cols("Status")
	case "Job":
		return cols("Completions")
	default:
		return cols("Status")
	}
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

func podCells(u *unstructured.Unstructured) []string {
	total := len(nestedSlice(u, "spec", "containers"))
	ready := 0
	var restarts int64
	for _, s := range nestedSlice(u, "status", "containerStatuses") {
		m, ok := s.(map[string]interface{})
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
		fmt.Sprintf("%d", restarts),
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
		fmt.Sprintf("%d", updated),
		fmt.Sprintf("%d", available),
	}
}

func daemonCells(u *unstructured.Unstructured) []string {
	return []string{
		fmt.Sprintf("%d", nestedInt(u, "status", "desiredNumberScheduled")),
		fmt.Sprintf("%d", nestedInt(u, "status", "numberReady")),
		fmt.Sprintf("%d", nestedInt(u, "status", "numberAvailable")),
	}
}

func serviceCells(u *unstructured.Unstructured) []string {
	parts := []string{}
	for _, p := range nestedSlice(u, "spec", "ports") {
		m, ok := p.(map[string]interface{})
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
		m, ok := c.(map[string]interface{})
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
		m, ok := c.(map[string]interface{})
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

func nestedSlice(u *unstructured.Unstructured, fields ...string) []interface{} {
	v, found, err := unstructured.NestedSlice(u.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return v
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}
