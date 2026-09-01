package issues

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAStatefulSetChangeNamesItsUpdateRevision(t *testing.T) {
	obj := newWorkload(kindStatefulSet, "db", "uid-db", map[string]any{
		"updateRevision": "db-6f9c4d8b7a",
		"conditions":     []any{condition("Ready", "False", nil)},
	}, map[string]any{})
	snap := newSnapshot()
	entry := object{obj: obj, desc: descriptor(appsGroup, "v1", "statefulsets", kindStatefulSet)}

	moved := changeOf(snap, entry)

	if moved.what != "revision db-6f9c4d8b7a" {
		t.Fatalf("change = %q, want the update revision", moved.what)
	}
	if !moved.at.Equal(testNow.Add(-30 * time.Minute)) {
		t.Fatalf("changedAt = %v, want the newest condition transition", moved.at)
	}
}

func TestAStatefulSetWithoutARevisionFallsBackToItsGeneration(t *testing.T) {
	obj := newWorkload(kindStatefulSet, "db", "uid-db", map[string]any{}, map[string]any{})
	snap := newSnapshot()
	entry := object{obj: obj, desc: descriptor(appsGroup, "v1", "statefulsets", kindStatefulSet)}

	if moved := changeOf(snap, entry); moved.what != "generation 3" {
		t.Fatalf("change = %q, want the generation", moved.what)
	}
}

func TestAnObjectWithoutAGenerationHasNoChangeToShow(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "db", "namespace": "default", "uid": "uid-db"},
	}}
	snap := newSnapshot()
	entry := object{obj: obj, desc: descriptor(appsGroup, "v1", "daemonsets", kindDaemonSet)}

	if moved := changeOf(snap, entry); moved.what != "" {
		t.Fatalf("change = %q, want nothing", moved.what)
	}
}

func TestAKindWithNoChangeToReportIsQuiet(t *testing.T) {
	obj := custom("Certificate", "wildcard", map[string]any{})
	snap := newSnapshot()
	entry := object{obj: obj, desc: certificateDescriptor()}

	if moved := changeOf(snap, entry); moved.what != "" {
		t.Fatalf("change = %q, want nothing for a kind with no rollout", moved.what)
	}
}

func TestAReplicaSetWithoutARevisionBorrowsItsDeploymentsOne(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{}, map[string]any{})
	deployment.SetAnnotations(map[string]string{revisionAnnotation: "9"})
	replica := newWorkload(kindReplicaSet, "web-abc", "uid-rs", map[string]any{}, map[string]any{})
	controller := true
	replica.SetOwnerReferences(ownerReference(kindDeployment, "web", "uid-web", &controller))
	snap := snapshotOf(
		object{obj: deployment, desc: deploymentDescriptor()},
		object{obj: replica, desc: replicaSetDescriptor()},
	)

	moved := changeOf(snap, snap.byUID["uid-rs"])

	if moved.what != "revision 9" {
		t.Fatalf("change = %q, want the deployment's revision", moved.what)
	}
}

func TestAnOrphanedReplicaSetSaysOnlyThatItWasCreated(t *testing.T) {
	replica := newWorkload(kindReplicaSet, "web-abc", "uid-rs", map[string]any{}, map[string]any{})
	snap := snapshotOf(object{obj: replica, desc: replicaSetDescriptor()})

	if moved := changeOf(snap, snap.byUID["uid-rs"]); moved.what != "replica set created" {
		t.Fatalf("change = %q, want the creation", moved.what)
	}
}

func TestADeploymentWithoutReplicaSetsFallsBackToItsGeneration(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{}, map[string]any{})
	snap := snapshotOf(object{obj: deployment, desc: deploymentDescriptor()})

	if moved := changeOf(snap, snap.byUID["uid-web"]); moved.what != "generation 3" {
		t.Fatalf("change = %q, want the generation", moved.what)
	}
}

func TestADeploymentIgnoresChildrenThatAreNotReplicaSets(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{}, map[string]any{})
	pod := newPod("web-pod", withOwner(kindDeployment, "web", "uid-web"))
	snap := snapshotOf(
		object{obj: deployment, desc: deploymentDescriptor()},
		object{obj: pod, desc: podDescriptor()},
	)

	if moved := changeOf(snap, snap.byUID["uid-web"]); moved.what != "generation 3" {
		t.Fatalf("change = %q, want the deployment generation", moved.what)
	}
}

func TestADeploymentPicksItsNewestReplicaSet(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{}, map[string]any{})
	older := replicaSet("web-old", "uid-old", "uid-web", "3")
	newer := replicaSet("web-new", "uid-new", "uid-web", "4")
	setNested(newer, testNow.Format(time.RFC3339), "metadata", "creationTimestamp")
	snap := snapshotOf(
		object{obj: deployment, desc: deploymentDescriptor()},
		object{obj: older, desc: replicaSetDescriptor()},
		object{obj: newer, desc: replicaSetDescriptor()},
	)

	if moved := changeOf(snap, snap.byUID["uid-web"]); moved.what != "revision 4" {
		t.Fatalf("change = %q, want the newest replica set", moved.what)
	}
}

func TestAJobChangeIsItsCreation(t *testing.T) {
	job := newWorkload(kindJob, "import", "uid-import", map[string]any{}, map[string]any{})
	snap := snapshotOf(object{obj: job, desc: descriptor(batchGroup, "v1", "jobs", kindJob)})

	if moved := changeOf(snap, snap.byUID["uid-import"]); moved.what != "job created" {
		t.Fatalf("change = %q, want the creation", moved.what)
	}
}

func TestAPodWithoutAnOwnerHasNoRolloutToShow(t *testing.T) {
	pod := newPod("bare")
	snap := snapshotOf(object{obj: pod, desc: podDescriptor()})

	if moved := rolloutOf(snap, snap.byUID["uid-bare"]); moved.what != "" {
		t.Fatalf("change = %q, want nothing", moved.what)
	}
}

func TestARevisionWithAShortDigestIsLeftWhole(t *testing.T) {
	if got := shortened("v1.2.3"); got != "v1.2.3" {
		t.Fatalf("shortened = %q, want the tag kept whole", got)
	}
}

func TestARevisionWithoutASeparatorIsStillShortened(t *testing.T) {
	if got := shortened("abcdef0123456789abcdef"); got != "abcdef0" {
		t.Fatalf("shortened = %q, want the short digest", got)
	}
}

func TestAConditionWithAnUnreadableTimeIsSkipped(t *testing.T) {
	obj := newWorkload(kindStatefulSet, "db", "uid-db", map[string]any{
		"conditions": []any{
			"not a condition",
			map[string]any{"type": "Ready", "status": "False", "lastTransitionTime": "not a time"},
		},
	}, map[string]any{})

	if got := transitionOf(obj); !got.Equal(testNow.Add(-time.Hour)) {
		t.Fatalf("transition = %v, want the creation time", got)
	}
}

func TestAReplicaSetWithNoRevisionAnywhereSaysOnlyThatItRolledOut(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{}, map[string]any{})
	replica := newWorkload(kindReplicaSet, "web-abc", "uid-rs", map[string]any{}, map[string]any{})
	controller := true
	replica.SetOwnerReferences(ownerReference(kindDeployment, "web", "uid-web", &controller))
	snap := snapshotOf(
		object{obj: deployment, desc: deploymentDescriptor()},
		object{obj: replica, desc: replicaSetDescriptor()},
	)

	moved := changeOf(snap, snap.byUID["uid-rs"])

	if moved.what != "rolled out" {
		t.Fatalf("change = %q, want the bare statement when no revision is recorded anywhere", moved.what)
	}
}
