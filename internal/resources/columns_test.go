package resources

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func u(obj map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: obj}
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cells = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cells = %v, want %v", got, want)
		}
	}
}

func colNames(t *testing.T, kind string) []string {
	t.Helper()
	cols := columnsFor(kind)
	names := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, c.Name)
	}
	return names
}

func TestColumnsFor(t *testing.T) {
	cases := []struct {
		kind string
		want []string
	}{
		{"Pod", []string{"Containers", "Status", "Restarts", "Node"}},
		{"Deployment", []string{"Ready", "Up-to-date", "Available"}},
		{"ReplicaSet", []string{"Ready", "Up-to-date", "Available"}},
		{"StatefulSet", []string{"Ready", "Up-to-date", "Available"}},
		{"ReplicationController", []string{"Ready", "Up-to-date", "Available"}},
		{"DaemonSet", []string{"Desired", "Ready", "Available"}},
		{"Service", []string{"Type", "Cluster-IP", "Ports"}},
		{"Node", []string{"Status", "Roles", "Version"}},
		{"Namespace", []string{"Status"}},
		{"Job", []string{"Completions"}},
		{"ConfigMap", []string{"Status"}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			eq(t, colNames(t, tc.kind), tc.want)
		})
	}
}

func TestColumnRenders(t *testing.T) {
	pod := columnsFor("Pod")
	if pod[0].Render != "containers" {
		t.Fatalf("pod[0] render = %q, want containers", pod[0].Render)
	}
	if pod[2].Render != "restarts" {
		t.Fatalf("pod[2] render = %q, want restarts", pod[2].Render)
	}
	if columnsFor("Deployment")[0].Render != "ratio" {
		t.Fatalf("deployment[0] render, want ratio")
	}
	if columnsFor("Job")[0].Render != "ratio" {
		t.Fatalf("job[0] render, want ratio")
	}
	if columnsFor("ConfigMap")[0].Render != "status" {
		t.Fatalf("configmap[0] render, want status")
	}
	if columnsFor("Node")[0].Render != "status" {
		t.Fatalf("node[0] render, want status")
	}
	if columnsFor("Node")[1].Render != "" {
		t.Fatalf("node[1] render, want empty")
	}
	if columnsFor("Namespace")[0].Render != "status" {
		t.Fatalf("namespace[0] render, want status")
	}
	if columnsFor("Pod")[1].Render != "status" {
		t.Fatalf("pod[1] render, want status")
	}
}

func TestContainersForNonPod(t *testing.T) {
	if got := containersFor(u(map[string]any{}), "Deployment"); got != nil {
		t.Fatalf("containersFor non-pod = %v, want nil", got)
	}
}

func TestContainersForEmptyPod(t *testing.T) {
	if got := containersFor(u(map[string]any{}), "Pod"); got != nil {
		t.Fatalf("containersFor empty pod = %v, want nil", got)
	}
}

func TestContainersForPod(t *testing.T) {
	pod := u(map[string]any{
		"status": map[string]any{
			"initContainerStatuses": []any{
				map[string]any{
					"name":         "init",
					"ready":        true,
					"restartCount": int64(0),
					"state":        map[string]any{"terminated": map[string]any{"reason": "Completed"}},
				},
			},
			"containerStatuses": []any{
				"not-a-map",
				map[string]any{
					"name":         "app",
					"ready":        true,
					"restartCount": int64(1),
					"state":        map[string]any{"running": map[string]any{}},
				},
				map[string]any{
					"name":         "sidecar",
					"ready":        false,
					"restartCount": int64(4),
					"state":        map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}},
				},
			},
		},
	})
	states := containersFor(pod, "Pod")
	if len(states) != 3 {
		t.Fatalf("states = %d, want 3", len(states))
	}
	if !states[0].Init || states[0].Name != "init" || states[0].State != "terminated" || states[0].Reason != "Completed" {
		t.Fatalf("init state = %+v", states[0])
	}
	if states[1].Init || states[1].Name != "app" || states[1].State != "running" || !states[1].Ready || states[1].Restarts != 1 {
		t.Fatalf("app state = %+v", states[1])
	}
	if states[2].Name != "sidecar" || states[2].State != "waiting" || states[2].Reason != "CrashLoopBackOff" || states[2].Ready {
		t.Fatalf("sidecar state = %+v", states[2])
	}
}

func TestContainerStateReason(t *testing.T) {
	if state, reason := containerStateReason(map[string]any{}); state != "waiting" || reason != "" {
		t.Fatalf("no state = %q,%q, want waiting,''", state, reason)
	}
	empty := map[string]any{"state": map[string]any{}}
	if state, reason := containerStateReason(empty); state != "waiting" || reason != "" {
		t.Fatalf("empty state = %q,%q, want waiting,''", state, reason)
	}
}

func TestPodCells(t *testing.T) {
	pod := u(map[string]any{
		"spec": map[string]any{
			"nodeName": "node-1",
			"containers": []any{
				map[string]any{"name": "a"},
				map[string]any{"name": "b"},
			},
		},
		"status": map[string]any{
			"phase": "Running",
			"containerStatuses": []any{
				map[string]any{"ready": true, "restartCount": int64(2)},
				map[string]any{"ready": false, "restartCount": int64(3)},
			},
		},
	})
	eq(t, podCells(pod), []string{"1/2", "Running", "5", "node-1"})
}

func TestPodCellsEmpty(t *testing.T) {
	pod := u(map[string]any{})
	eq(t, podCells(pod), []string{"0/0", "", "0", ""})
}

func TestPodCellsSkipsNonMapContainerStatus(t *testing.T) {
	pod := u(map[string]any{
		"spec": map[string]any{
			"containers": []any{map[string]any{"name": "a"}},
		},
		"status": map[string]any{
			"phase": "Running",
			"containerStatuses": []any{
				"not-a-map",
				map[string]any{"ready": true, "restartCount": int64(1)},
			},
		},
	})
	eq(t, podCells(pod), []string{"1/1", "Running", "1", ""})
}

func TestWorkloadCells(t *testing.T) {
	dep := u(map[string]any{
		"spec": map[string]any{"replicas": int64(3)},
		"status": map[string]any{
			"readyReplicas":     int64(2),
			"updatedReplicas":   int64(3),
			"availableReplicas": int64(2),
		},
	})
	eq(t, workloadCells(dep), []string{"2/3", "3", "2"})
}

func TestWorkloadCellsMissing(t *testing.T) {
	dep := u(map[string]any{})
	eq(t, workloadCells(dep), []string{"0/0", "0", "0"})
}

func TestDaemonCells(t *testing.T) {
	ds := u(map[string]any{
		"status": map[string]any{
			"desiredNumberScheduled": int64(4),
			"numberReady":            int64(3),
			"numberAvailable":        int64(3),
		},
	})
	eq(t, daemonCells(ds), []string{"4", "3", "3"})
}

func TestServiceCells(t *testing.T) {
	svc := u(map[string]any{
		"spec": map[string]any{
			"type":      "ClusterIP",
			"clusterIP": "10.0.0.1",
			"ports": []any{
				map[string]any{"port": int64(80), "protocol": "TCP"},
				map[string]any{"port": int64(443), "protocol": "TCP"},
			},
		},
	})
	eq(t, serviceCells(svc), []string{"ClusterIP", "10.0.0.1", "80/TCP,443/TCP"})
}

func TestServiceCellsPortWithoutProtocol(t *testing.T) {
	svc := u(map[string]any{
		"spec": map[string]any{
			"type":      "NodePort",
			"clusterIP": "10.0.0.2",
			"ports": []any{
				"not-a-map",
				map[string]any{"port": int64(8080)},
			},
		},
	})
	eq(t, serviceCells(svc), []string{"NodePort", "10.0.0.2", "8080/"})
}

func TestNodeCellsReady(t *testing.T) {
	node := u(map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{
				"node-role.kubernetes.io/control-plane": "",
				"node-role.kubernetes.io/worker":        "",
				"kubernetes.io/hostname":                "node-1",
			},
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "MemoryPressure", "status": "False"},
				map[string]any{"type": "Ready", "status": "True"},
			},
			"nodeInfo": map[string]any{"kubeletVersion": "v1.31.0"},
		},
	})
	cells := nodeCells(node)
	if cells[0] != "Ready" {
		t.Fatalf("status = %q, want Ready", cells[0])
	}
	if cells[2] != "v1.31.0" {
		t.Fatalf("version = %q, want v1.31.0", cells[2])
	}
	roles := cells[1]
	if roles != "control-plane,worker" && roles != "worker,control-plane" {
		t.Fatalf("roles = %q, want control-plane and worker", roles)
	}
}

func TestNodeCellsNotReady(t *testing.T) {
	node := u(map[string]any{
		"status": map[string]any{
			"conditions": []any{
				"not-a-map",
				map[string]any{"type": "Ready", "status": "False"},
			},
		},
	})
	eq(t, nodeCells(node), []string{"NotReady", "", ""})
}

func TestNodeCellsSkipsEmptyRole(t *testing.T) {
	node := u(map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{
				"node-role.kubernetes.io/": "",
			},
		},
		"status": map[string]any{},
	})
	eq(t, nodeCells(node), []string{"NotReady", "", ""})
}

func TestJobCells(t *testing.T) {
	job := u(map[string]any{
		"spec":   map[string]any{"completions": int64(5)},
		"status": map[string]any{"succeeded": int64(3)},
	})
	eq(t, jobCells(job), []string{"3/5"})
}

func TestConditionSummaryReady(t *testing.T) {
	obj := u(map[string]any{
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	})
	if got := conditionSummary(obj); got != "Ready" {
		t.Fatalf("conditionSummary = %q, want Ready", got)
	}
}

func TestConditionSummaryReason(t *testing.T) {
	obj := u(map[string]any{
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Available", "status": "False"},
				map[string]any{"type": "Ready", "status": "False", "reason": "MinimumReplicasUnavailable"},
			},
		},
	})
	if got := conditionSummary(obj); got != "MinimumReplicasUnavailable" {
		t.Fatalf("conditionSummary = %q, want MinimumReplicasUnavailable", got)
	}
}

func TestConditionSummaryNotReady(t *testing.T) {
	obj := u(map[string]any{
		"status": map[string]any{
			"conditions": []any{
				"not-a-map",
				map[string]any{"type": "Ready", "status": "False"},
			},
		},
	})
	if got := conditionSummary(obj); got != "NotReady" {
		t.Fatalf("conditionSummary = %q, want NotReady", got)
	}
}

func TestConditionSummaryEmpty(t *testing.T) {
	obj := u(map[string]any{})
	if got := conditionSummary(obj); got != "" {
		t.Fatalf("conditionSummary = %q, want empty", got)
	}
}

func TestCellsForDispatch(t *testing.T) {
	pod := u(map[string]any{
		"spec":   map[string]any{"containers": []any{map[string]any{"name": "a"}}},
		"status": map[string]any{"phase": "Running"},
	})
	eq(t, cellsFor(pod, "Pod"), []string{"0/1", "Running", "0", ""})

	dep := u(map[string]any{
		"spec":   map[string]any{"replicas": int64(1)},
		"status": map[string]any{"readyReplicas": int64(1), "updatedReplicas": int64(1), "availableReplicas": int64(1)},
	})
	eq(t, cellsFor(dep, "Deployment"), []string{"1/1", "1", "1"})

	ds := u(map[string]any{
		"status": map[string]any{"desiredNumberScheduled": int64(1), "numberReady": int64(1), "numberAvailable": int64(1)},
	})
	eq(t, cellsFor(ds, "DaemonSet"), []string{"1", "1", "1"})

	svc := u(map[string]any{
		"spec": map[string]any{"type": "ClusterIP", "clusterIP": "10.0.0.1"},
	})
	eq(t, cellsFor(svc, "Service"), []string{"ClusterIP", "10.0.0.1", ""})

	node := u(map[string]any{"status": map[string]any{}})
	eq(t, cellsFor(node, "Node"), []string{"NotReady", "", ""})

	ns := u(map[string]any{"status": map[string]any{"phase": "Active"}})
	eq(t, cellsFor(ns, "Namespace"), []string{"Active"})

	job := u(map[string]any{
		"spec":   map[string]any{"completions": int64(1)},
		"status": map[string]any{"succeeded": int64(1)},
	})
	eq(t, cellsFor(job, "Job"), []string{"1/1"})

	generic := u(map[string]any{
		"status": map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "True"}}},
	})
	eq(t, cellsFor(generic, "ConfigMap"), []string{"Ready"})
}

func TestToInt64(t *testing.T) {
	if got := toInt64(int64(7)); got != 7 {
		t.Fatalf("toInt64(int64) = %d, want 7", got)
	}
	if got := toInt64(float64(9)); got != 9 {
		t.Fatalf("toInt64(float64) = %d, want 9", got)
	}
	if got := toInt64("nope"); got != 0 {
		t.Fatalf("toInt64(string) = %d, want 0", got)
	}
	if got := toInt64(nil); got != 0 {
		t.Fatalf("toInt64(nil) = %d, want 0", got)
	}
}

func TestNestedHelpersOnWrongType(t *testing.T) {
	obj := u(map[string]any{
		"spec": map[string]any{
			"replicas": "three",
			"nodeName": int64(5),
			"ports":    "not-a-slice",
		},
	})
	if got := nestedString(obj, "spec", "nodeName"); got != "" {
		t.Fatalf("nestedString on non-string = %q, want empty", got)
	}
	if got := nestedInt(obj, "spec", "replicas"); got != 0 {
		t.Fatalf("nestedInt on non-int = %d, want 0", got)
	}
	if got := nestedSlice(obj, "spec", "ports"); got != nil {
		t.Fatalf("nestedSlice on non-slice = %v, want nil", got)
	}
}

func TestContainersForMarksEphemeralContainers(t *testing.T) {
	pod := u(map[string]any{
		"status": map[string]any{
			"containerStatuses": []any{
				map[string]any{"name": "app", "ready": true, "restartCount": int64(0), "state": map[string]any{"running": map[string]any{}}},
			},
			"ephemeralContainerStatuses": []any{
				map[string]any{"name": "spinoza-debug-1", "ready": false, "restartCount": int64(0), "state": map[string]any{"running": map[string]any{}}},
			},
		},
	})
	states := containersFor(pod, "Pod")
	if len(states) != 2 {
		t.Fatalf("states = %d, want 2", len(states))
	}
	if states[0].Name != "app" || states[0].Ephemeral {
		t.Fatalf("regular container = %+v", states[0])
	}
	if states[1].Name != "spinoza-debug-1" || !states[1].Ephemeral {
		t.Fatalf("debug container = %+v, want Ephemeral true", states[1])
	}
	if states[1].Init {
		t.Fatal("an ephemeral container is not an init container")
	}
}

func TestNodeCellsMarkACordonedNode(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "worker-1"},
		"spec":       map[string]any{"unschedulable": true},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	}}

	if got := nodeCells(node)[0]; got != "Ready,SchedulingDisabled" {
		t.Fatalf("status = %q", got)
	}
}

func TestNodeCellsLeaveASchedulableNodeAlone(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "worker-1"},
		"spec":       map[string]any{"unschedulable": false},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	}}

	if got := nodeCells(node)[0]; got != "Ready" {
		t.Fatalf("status = %q", got)
	}
}

func TestNodeCellsIgnoreANonBoolUnschedulable(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "worker-1"},
		"spec":       map[string]any{"unschedulable": "yes"},
		"status":     map[string]any{"conditions": []any{}},
	}}

	if got := nodeCells(node)[0]; got != "NotReady" {
		t.Fatalf("status = %q", got)
	}
}
