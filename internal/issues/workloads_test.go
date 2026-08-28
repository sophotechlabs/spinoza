package issues

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func deploymentWith(name string, status, spec map[string]any) *unstructured.Unstructured {
	return newWorkload(kindDeployment, name, "uid-"+name, status, spec)
}

func deploymentItems(objs ...*unstructured.Unstructured) map[string][]*unstructured.Unstructured {
	return map[string][]*unstructured.Unstructured{"deployments": objs}
}

func TestAQuotaRejectionIsFatalAndNamed(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions": []any{condition("ReplicaFailure", "True", map[string]any{
			"reason":  "FailedCreate",
			"message": `pods "web-abc" is forbidden: exceeded quota: team, requested: cpu=1`,
		})},
	}, map[string]any{"replicas": int64(3)})
	lister := &stubLister{items: deploymentItems(deployment)}

	queue := build(t, lister, catalog(deploymentDescriptor()))

	row, ok := rowNamed(queue, "web")
	if !ok || row.Title != "BlockedByQuota" || row.Severity != api.SeverityFatal {
		t.Fatalf("row = %+v, want a fatal quota row", row)
	}
}

func TestAWebhookRejectionIsNamed(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"conditions": []any{condition("ReplicaFailure", "True", map[string]any{
			"message": "admission webhook \"vpod.kb.io\" denied the request",
		})},
	}, map[string]any{"replicas": int64(1)})
	lister := &stubLister{items: deploymentItems(deployment)}

	row, _ := rowNamed(build(t, lister, catalog(deploymentDescriptor())), "web")
	if row.Title != "BlockedByWebhook" {
		t.Fatalf("title = %q, want the webhook", row.Title)
	}
}

func TestAPodSecurityRejectionIsNamed(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"conditions": []any{condition("ReplicaFailure", "True", map[string]any{
			"message": "violates PodSecurity \"restricted:latest\"",
		})},
	}, map[string]any{"replicas": int64(1)})
	lister := &stubLister{items: deploymentItems(deployment)}

	row, _ := rowNamed(build(t, lister, catalog(deploymentDescriptor())), "web")
	if row.Title != "BlockedByPodSecurity" {
		t.Fatalf("title = %q, want pod security", row.Title)
	}
}

func TestAReplicaFailureFallsBackToItsReason(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"conditions": []any{condition("ReplicaFailure", "True", map[string]any{"reason": "FailedCreate"})},
	}, map[string]any{"replicas": int64(1)})
	lister := &stubLister{items: deploymentItems(deployment)}

	row, _ := rowNamed(build(t, lister, catalog(deploymentDescriptor())), "web")
	if row.Title != "FailedCreate" || !contains(row.Detail, "could not create") {
		t.Fatalf("row = %+v, want the reason and the fallback message", row)
	}
}

func TestAReplicaFailureWithoutAReasonIsStillReported(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"conditions": []any{condition("ReplicaFailure", "True", nil)},
	}, map[string]any{"replicas": int64(1)})
	lister := &stubLister{items: deploymentItems(deployment)}

	row, _ := rowNamed(build(t, lister, catalog(deploymentDescriptor())), "web")
	if row.Title != "ReplicaFailure" {
		t.Fatalf("title = %q, want the bare condition name", row.Title)
	}
}

func TestAStalledRolloutWithNothingAvailableIsFatal(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"availableReplicas": int64(0),
		"conditions": []any{condition("Progressing", "False", map[string]any{
			"reason":  "ProgressDeadlineExceeded",
			"message": "ReplicaSet \"web-7\" has timed out progressing",
		})},
	}, map[string]any{"replicas": int64(3)})
	lister := &stubLister{items: deploymentItems(deployment)}

	row, _ := rowNamed(build(t, lister, catalog(deploymentDescriptor())), "web")
	if row.Severity != api.SeverityFatal || row.Title != "ProgressDeadlineExceeded" {
		t.Fatalf("row = %+v, want a fatal stalled rollout", row)
	}
}

func TestAStalledRolloutWithSomethingServingIsDegraded(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"availableReplicas": int64(2),
		"conditions":        []any{condition("Progressing", "False", nil)},
	}, map[string]any{"replicas": int64(3)})
	lister := &stubLister{items: deploymentItems(deployment)}

	row, _ := rowNamed(build(t, lister, catalog(deploymentDescriptor())), "web")
	if row.Severity != api.SeverityDegraded || row.Title != "RolloutStalled" {
		t.Fatalf("row = %+v, want a degraded stalled rollout", row)
	}
	if !contains(row.Detail, "stopped making progress") {
		t.Fatalf("detail = %q, want the fallback message", row.Detail)
	}
}

func TestReplicasShortOfDesiredAreReportedAfterTheGrace(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(1),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(3)})
	lister := &stubLister{items: deploymentItems(deployment)}

	row, _ := rowNamed(build(t, lister, catalog(deploymentDescriptor())), "web")
	if row.Title != "NotEnoughReplicas" || !contains(row.Detail, "1 of 3") {
		t.Fatalf("row = %+v, want the replica shortfall", row)
	}
	if row.Severity != api.SeverityDegraded {
		t.Fatalf("severity = %q, want degraded while one replica serves", row.Severity)
	}
}

func TestNoReplicasReadyIsFatal(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(2)})
	lister := &stubLister{items: deploymentItems(deployment)}

	row, _ := rowNamed(build(t, lister, catalog(deploymentDescriptor())), "web")
	if row.Severity != api.SeverityFatal {
		t.Fatalf("severity = %q, want fatal when nothing is ready", row.Severity)
	}
}

func TestAFreshRolloutIsGivenItsGrace(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions": []any{map[string]any{
			"type":               "Available",
			"status":             "True",
			"lastTransitionTime": testNow.Add(-30 * time.Second).Format(time.RFC3339),
		}},
	}, map[string]any{"replicas": int64(2)})
	lister := &stubLister{items: deploymentItems(deployment)}

	if queue := build(t, lister, catalog(deploymentDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want the rollout left alone inside the grace", queue.Rows)
	}
}

func TestAHealthyDeploymentIsNotAnIssue(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(3),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(3)})
	lister := &stubLister{items: deploymentItems(deployment)}

	if queue := build(t, lister, catalog(deploymentDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestADeletedWorkloadIsNotAnIssue(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{"readyReplicas": int64(0)}, map[string]any{"replicas": int64(3)})
	stamp := metaNow()
	deployment.SetDeletionTimestamp(&stamp)
	lister := &stubLister{items: deploymentItems(deployment)}

	if queue := build(t, lister, catalog(deploymentDescriptor())); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none while it is going away", queue.Rows)
	}
}

func TestADaemonSetCountsScheduledNodes(t *testing.T) {
	daemon := newWorkload(kindDaemonSet, "agent", "uid-agent", map[string]any{
		"numberReady":            int64(2),
		"desiredNumberScheduled": int64(5),
	}, map[string]any{})
	desc := descriptor(appsGroup, "v1", "daemonsets", kindDaemonSet)
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"daemonsets": {daemon}}}

	row, _ := rowNamed(build(t, lister, catalog(desc)), "agent")
	if !contains(row.Detail, "2 of 5") {
		t.Fatalf("detail = %q, want the daemon set counts", row.Detail)
	}
}

func TestAFailedJobIsFatal(t *testing.T) {
	job := newWorkload(kindJob, "import", "uid-import", map[string]any{
		"conditions": []any{condition("Failed", "True", map[string]any{
			"message": "Job has reached the specified backoff limit",
		})},
	}, map[string]any{})
	desc := descriptor(batchGroup, "v1", "jobs", kindJob)
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"jobs": {job}}}

	row, _ := rowNamed(build(t, lister, catalog(desc)), "import")
	if row.Title != "JobFailed" || row.Severity != api.SeverityFatal {
		t.Fatalf("row = %+v, want a fatal job row", row)
	}
	if !contains(row.Detail, "backoff limit") {
		t.Fatalf("detail = %q, want the job message", row.Detail)
	}
}

func TestAFailedJobWithoutAMessageStillReports(t *testing.T) {
	job := newWorkload(kindJob, "import", "uid-import", map[string]any{
		"conditions": []any{condition("Failed", "True", nil)},
	}, map[string]any{})
	desc := descriptor(batchGroup, "v1", "jobs", kindJob)
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"jobs": {job}}}

	row, _ := rowNamed(build(t, lister, catalog(desc)), "import")
	if !contains(row.Detail, "gave up") {
		t.Fatalf("detail = %q, want the fallback", row.Detail)
	}
}

func TestARunningJobIsNotAnIssue(t *testing.T) {
	job := newWorkload(kindJob, "import", "uid-import", map[string]any{
		"conditions": []any{condition("Failed", "False", nil)},
	}, map[string]any{})
	desc := descriptor(batchGroup, "v1", "jobs", kindJob)
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{"jobs": {job}}}

	if queue := build(t, lister, catalog(desc)); len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", queue.Rows)
	}
}

func TestWorkloadUnhealthyKeepsTheBadgeDefinition(t *testing.T) {
	cases := []struct {
		name string
		kind string
		obj  *unstructured.Unstructured
		want bool
	}{
		{
			name: "a deployment short of replicas",
			kind: kindDeployment,
			obj:  deploymentWith("web", map[string]any{"readyReplicas": int64(1)}, map[string]any{"replicas": int64(3)}),
			want: true,
		},
		{
			name: "a deployment at its replica count",
			kind: kindDeployment,
			obj:  deploymentWith("web", map[string]any{"readyReplicas": int64(3)}, map[string]any{"replicas": int64(3)}),
			want: false,
		},
		{
			name: "a daemon set short of nodes",
			kind: kindDaemonSet,
			obj: newWorkload(kindDaemonSet, "agent", "uid", map[string]any{
				"numberReady":            int64(1),
				"desiredNumberScheduled": int64(4),
			}, map[string]any{}),
			want: true,
		},
		{
			name: "a failed job",
			kind: kindJob,
			obj: newWorkload(kindJob, "import", "uid", map[string]any{
				"conditions": []any{condition("Failed", "True", nil)},
			}, map[string]any{}),
			want: true,
		},
		{
			name: "a kind the badge does not judge",
			kind: "ConfigMap",
			obj:  newWorkload("ConfigMap", "settings", "uid", map[string]any{}, map[string]any{}),
			want: false,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := WorkloadUnhealthy(item.obj, item.kind); got != item.want {
				t.Fatalf("WorkloadUnhealthy = %v, want %v", got, item.want)
			}
		})
	}
}

func TestAConditionThatIsNotAMapIsSkipped(t *testing.T) {
	obj := newWorkload(kindJob, "import", "uid", map[string]any{
		"conditions": []any{
			"not a condition",
			map[string]any{"type": "Suspended", "status": "False"},
			map[string]any{"type": "Failed", "status": "True"},
		},
	}, map[string]any{})

	if !conditionIsTrue(obj, "Failed") {
		t.Fatal("failed = false, want the condition found past the noise")
	}
}

func TestAMissingConditionIsNotTrue(t *testing.T) {
	obj := newWorkload(kindJob, "import", "uid", map[string]any{}, map[string]any{})

	if conditionIsTrue(obj, "Failed") {
		t.Fatal("failed = true, want false when the condition is absent")
	}
}
