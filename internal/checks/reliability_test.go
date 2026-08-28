package checks

import (
	"strings"
	"testing"
)

func TestAContainerWithNeitherProbeIsFlagged(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", nil))))

	if onlyFinding(t, found, "probes-missing").Detail != "no livenessProbe and no readinessProbe" {
		t.Fatal("a container with no probes was not reported")
	}
}

func TestEitherProbeIsEnough(t *testing.T) {
	ready := report(t, deployment("api", podSpec(container("app", map[string]any{
		"readinessProbe": map[string]any{"tcpSocket": map[string]any{"port": int64(8080)}},
	}))))
	if findingCount(t, ready, "probes-missing") != 0 {
		t.Fatal("a readinessProbe did not satisfy the check")
	}

	live := report(t, deployment("api", podSpec(container("app", map[string]any{
		"livenessProbe": map[string]any{"tcpSocket": map[string]any{"port": int64(8080)}},
	}))))
	if findingCount(t, live, "probes-missing") != 0 {
		t.Fatal("a livenessProbe did not satisfy the check")
	}
}

func TestJobsAndInitContainersAreNotAskedForProbes(t *testing.T) {
	job := report(t, workload("Job", "migrate", podSpec(container("app", nil))))
	if findingCount(t, job, "probes-missing") != 0 {
		t.Fatal("a Job was asked for probes")
	}

	scheduled := report(t, cronJob("nightly", podSpec(container("app", nil))))
	if findingCount(t, scheduled, "probes-missing") != 0 {
		t.Fatal("a CronJob was asked for probes")
	}

	spec := podSpec(container("app", map[string]any{
		"readinessProbe": map[string]any{"tcpSocket": map[string]any{"port": int64(8080)}},
	}))
	spec["initContainers"] = []any{container("setup", nil)}
	if findingCount(t, report(t, deployment("api", spec)), "probes-missing") != 0 {
		t.Fatal("an init container was asked for probes")
	}
}

func TestAPodThatRunsOnceIsNotAskedForProbes(t *testing.T) {
	spec := podSpec(container("app", nil))
	spec["restartPolicy"] = "Never"

	if findingCount(t, report(t, pod("oneshot", spec)), "probes-missing") != 0 {
		t.Fatal("a run-once pod was asked for probes")
	}
}

func TestMutableImageTagsAreFlagged(t *testing.T) {
	latest := report(t, deployment("api", podSpec(container("app", map[string]any{
		"image": "registry.example/app:latest",
	}))))
	if !strings.Contains(onlyFinding(t, latest, "image-latest").Detail, "is tagged :latest") {
		t.Fatal("an image tagged :latest was not reported")
	}

	bare := report(t, deployment("api", podSpec(container("app", map[string]any{
		"image": "registry.example:5000/app",
	}))))
	if !strings.Contains(onlyFinding(t, bare, "image-latest").Detail, "carries no tag") {
		t.Fatal("an untagged image behind a port was not reported")
	}
}

func TestPinnedImagesAreNotFlagged(t *testing.T) {
	for _, image := range []string{
		"registry.example/app:1.4.2",
		"registry.example:5000/app:1.4.2",
		"registry.example/app@sha256:0000000000000000000000000000000000000000000000000000000000000000",
	} {
		found := report(t, deployment("api", podSpec(container("app", map[string]any{"image": image}))))
		if findingCount(t, found, "image-latest") != 0 {
			t.Fatalf("%s was reported as mutable", image)
		}
	}
}

func TestAContainerWithNoImageIsNotFlagged(t *testing.T) {
	found := report(t, deployment("api", podSpec(map[string]any{"name": "app"})))

	if findingCount(t, found, "image-latest") != 0 {
		t.Fatal("a container with no image was reported")
	}
}

func TestASingleReplicaDeploymentIsFlagged(t *testing.T) {
	implicit := report(t, deployment("api", podSpec(container("app", nil))))
	finding := onlyFinding(t, implicit, "single-replica")
	if finding.Detail != "spec.replicas is 1" {
		t.Fatalf("detail was %q", finding.Detail)
	}
	if finding.Patch != "spec:\n  replicas: 2\n" {
		t.Fatalf("patch was:\n%s", finding.Patch)
	}

	scaled := report(t, replicas(deployment("api", podSpec(container("app", nil))), 3))
	if findingCount(t, scaled, "single-replica") != 0 {
		t.Fatal("a three-replica Deployment was reported")
	}
}

func TestOnlyDeploymentsAreAskedAboutReplicaCount(t *testing.T) {
	found := report(t, workload("StatefulSet", "db", podSpec(container("app", nil))))

	if findingCount(t, found, "single-replica") != 0 {
		t.Fatal("a StatefulSet was reported as a single-replica Deployment")
	}
}

func TestReplicasOnOneNodeAreFlaggedWithASpreadPatch(t *testing.T) {
	owner := replicas(deployment("api", podSpec(container("app", nil))), 2)
	first := ownedBy(onNode(pod("api-a", podSpec(container("app", nil))), "worker-1"), "Deployment", "api")
	second := ownedBy(onNode(pod("api-b", podSpec(container("app", nil))), "worker-1"), "Deployment", "api")

	finding := onlyFinding(t, report(t, owner, first, second), "replicas-one-node")

	if finding.Detail != "all 2 pods run on node worker-1" {
		t.Fatalf("detail was %q", finding.Detail)
	}
	if !strings.Contains(finding.Patch, "topologyKey: kubernetes.io/hostname") {
		t.Fatalf("patch was:\n%s", finding.Patch)
	}
	if !strings.Contains(finding.Patch, "app: api") {
		t.Fatalf("the workload's own selector did not reach the patch:\n%s", finding.Patch)
	}
}

func TestReplicasOnDifferentNodesAreNotFlagged(t *testing.T) {
	owner := replicas(deployment("api", podSpec(container("app", nil))), 2)
	first := ownedBy(onNode(pod("api-a", podSpec(container("app", nil))), "worker-1"), "Deployment", "api")
	second := ownedBy(onNode(pod("api-b", podSpec(container("app", nil))), "worker-2"), "Deployment", "api")

	if findingCount(t, report(t, owner, first, second), "replicas-one-node") != 0 {
		t.Fatal("replicas spread over two nodes were reported")
	}
}

func TestOneReplicaAndUnscheduledPodsAreNotASpreadFinding(t *testing.T) {
	single := deployment("api", podSpec(container("app", nil)))
	one := ownedBy(onNode(pod("api-a", podSpec(container("app", nil))), "worker-1"), "Deployment", "api")
	if findingCount(t, report(t, single, one), "replicas-one-node") != 0 {
		t.Fatal("a workload with one pod was reported")
	}

	owner := replicas(deployment("web", podSpec(container("app", nil))), 2)
	first := ownedBy(pod("web-a", podSpec(container("app", nil))), "Deployment", "web")
	second := ownedBy(pod("web-b", podSpec(container("app", nil))), "Deployment", "web")
	if findingCount(t, report(t, owner, first, second), "replicas-one-node") != 0 {
		t.Fatal("pods with no node were reported as sharing one")
	}
}

func TestADaemonSetIsNotAskedToSpread(t *testing.T) {
	owner := workload("DaemonSet", "agent", podSpec(container("app", nil)))
	first := ownedBy(onNode(pod("agent-a", podSpec(container("app", nil))), "worker-1"), "DaemonSet", "agent")
	second := ownedBy(onNode(pod("agent-b", podSpec(container("app", nil))), "worker-1"), "DaemonSet", "agent")

	if findingCount(t, report(t, owner, first, second), "replicas-one-node") != 0 {
		t.Fatal("a DaemonSet was asked to spread its pods")
	}
}

func TestASpreadPatchWithoutASelectorStillRenders(t *testing.T) {
	owner := workload("Job", "batch", podSpec(container("app", nil)))
	delete(specOf(owner), "selector")
	first := ownedBy(onNode(pod("batch-a", podSpec(container("app", nil))), "worker-1"), "Job", "batch")
	second := ownedBy(onNode(pod("batch-b", podSpec(container("app", nil))), "worker-1"), "Job", "batch")

	finding := onlyFinding(t, report(t, owner, first, second), "replicas-one-node")

	if strings.Contains(finding.Patch, "labelSelector") {
		t.Fatalf("a selector was invented:\n%s", finding.Patch)
	}
}

func TestAnImageWithNoRegistryAndNoTagIsFlagged(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", map[string]any{"image": "app"}))))

	if !strings.Contains(onlyFinding(t, found, "image-latest").Detail, "carries no tag") {
		t.Fatal("a bare image name was not reported as untagged")
	}
}

func TestASelectorThatIsNotAMapIsNotReadAsLabels(t *testing.T) {
	owner := workload("Job", "batch", podSpec(container("app", nil)))
	spec := specOf(owner)
	spec["selector"] = "not a map"
	first := ownedBy(onNode(pod("batch-a", podSpec(container("app", nil))), "worker-1"), "Job", "batch")
	second := ownedBy(onNode(pod("batch-b", podSpec(container("app", nil))), "worker-1"), "Job", "batch")

	finding := onlyFinding(t, report(t, owner, first, second), "replicas-one-node")

	if strings.Contains(finding.Patch, "labelSelector") {
		t.Fatalf("an unreadable selector reached the patch:\n%s", finding.Patch)
	}
}

func TestOnlyStringLabelsReachTheSpreadPatch(t *testing.T) {
	owner := workload("Job", "batch", podSpec(container("app", nil)))
	spec := specOf(owner)
	spec["selector"] = map[string]any{"matchLabels": map[string]any{"app": "batch", "tier": int64(2)}}
	first := ownedBy(onNode(pod("batch-a", podSpec(container("app", nil))), "worker-1"), "Job", "batch")
	second := ownedBy(onNode(pod("batch-b", podSpec(container("app", nil))), "worker-1"), "Job", "batch")

	finding := onlyFinding(t, report(t, owner, first, second), "replicas-one-node")

	if !strings.Contains(finding.Patch, "app: batch") {
		t.Fatalf("the string label did not reach the patch:\n%s", finding.Patch)
	}
	if strings.Contains(finding.Patch, "tier") {
		t.Fatalf("a non-string label reached the patch:\n%s", finding.Patch)
	}
}

func TestASelectorWithoutMatchLabelsIsSkipped(t *testing.T) {
	owner := workload("Job", "batch", podSpec(container("app", nil)))
	spec := specOf(owner)
	spec["selector"] = map[string]any{"matchExpressions": []any{}}
	first := ownedBy(onNode(pod("batch-a", podSpec(container("app", nil))), "worker-1"), "Job", "batch")
	second := ownedBy(onNode(pod("batch-b", podSpec(container("app", nil))), "worker-1"), "Job", "batch")

	finding := onlyFinding(t, report(t, owner, first, second), "replicas-one-node")

	if strings.Contains(finding.Patch, "labelSelector") {
		t.Fatalf("matchExpressions were read as matchLabels:\n%s", finding.Patch)
	}
}
