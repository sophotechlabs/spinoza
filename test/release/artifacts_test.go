package release_test

import (
	"slices"
	"strings"
	"testing"
)

func TestReleaseArtifactsRunOnEveryMainPush(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	if len(workflow.On.Push.Paths) != 0 {
		t.Fatalf("release artifact pushes are restricted to %v", workflow.On.Push.Paths)
	}
}

func TestReleaseDetectionCanSeeDrafts(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	version := requireJob(t, workflow, "version")
	if version.Permissions["contents"] != "write" {
		t.Fatal("release detection cannot see draft releases")
	}
}

func TestReleaseArtifactVersionContract(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	version := requireJob(t, workflow, "version")
	for _, output := range []string{"version", "pending", "sha"} {
		if version.Outputs[output] == "" {
			t.Errorf("version job has no %s output", output)
		}
	}
	read := requireStep(t, version, "read")
	if !strings.Contains(read.Run, "scripts/release-pending.sh") {
		t.Fatal("version job does not ask the release-pending checker")
	}
	if !strings.Contains(read.Run, "scripts/release-commit.sh") {
		t.Fatal("version job builds the dispatch head instead of the release commit")
	}
}

func TestReleaseArtifactBuildsAreGatedOnPendingWork(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	jobs := []string{"dist", "image", "chart", "desktop", "desktop-linux", "cluster-mode-release", "publish", "install"}
	for _, name := range jobs {
		job := requireJob(t, workflow, name)
		if !strings.Contains(job.If, "pending") {
			t.Errorf("%s job is not gated on pending release work", name)
		}
	}
}

func TestPublishedClusterModeCanUpgradeAndRollBackBeforeRelease(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	clusterMode := requireJob(t, workflow, "cluster-mode-release")
	if !contains(clusterMode.Needs, "chart") {
		t.Fatal("cluster-mode release validation does not wait for the published chart")
	}
	if clusterMode.Permissions["packages"] != "read" {
		t.Fatal("cluster-mode release validation cannot pull the published packages")
	}
	previous := requireStep(t, clusterMode, "previous")
	if !strings.Contains(previous.Run, "releases/latest") {
		t.Fatal("cluster-mode release validation does not resolve the previous stable release")
	}
	if !containsRun(clusterMode.Steps, "helm registry login") {
		t.Fatal("cluster-mode release validation does not authenticate to the chart registry")
	}
	if !containsRun(clusterMode.Steps, "test-cluster-mode-release") {
		t.Fatal("cluster-mode release validation does not run the lifecycle test")
	}
	lifecycle := requireRunStep(t, clusterMode, "test-cluster-mode-release")
	if lifecycle.Env["PREVIOUS"] != "${{ steps.previous.outputs.version }}" {
		t.Fatalf("previous release = %q", lifecycle.Env["PREVIOUS"])
	}
	if lifecycle.Env["CURRENT"] != "${{ needs.version.outputs.version }}" {
		t.Fatalf("current release = %q", lifecycle.Env["CURRENT"])
	}
	requireClusterModeFailureHandling(t, clusterMode)
	publish := requireJob(t, workflow, "publish")
	if !contains(publish.Needs, "cluster-mode-release") {
		t.Fatal("release publication does not wait for cluster-mode lifecycle validation")
	}
}

func TestClusterModeBrowsersRunEveryEngine(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/e2e.yaml")
	browser := requireJob(t, workflow, "cluster-mode-browser")
	matrix, ok := browser.Strategy["matrix"].(map[string]any)
	if !ok {
		t.Fatalf("cluster-mode matrix = %#v, want a mapping", browser.Strategy["matrix"])
	}
	rawBrowsers, ok := matrix["browser"].([]any)
	if !ok {
		t.Fatalf("cluster-mode browsers = %#v, want a list", matrix["browser"])
	}
	got := make([]string, 0, len(rawBrowsers))
	for _, raw := range rawBrowsers {
		name, isString := raw.(string)
		if !isString {
			t.Fatalf("cluster-mode browser = %#v, want a string", raw)
		}
		got = append(got, name)
	}
	want := []string{"chromium", "firefox", "webkit"}
	if !slices.Equal(got, want) {
		t.Fatalf("cluster-mode browsers = %v, want %v", got, want)
	}
	failFast, ok := browser.Strategy["fail-fast"].(bool)
	if !ok {
		t.Fatalf("cluster-mode fail-fast = %#v, want a boolean", browser.Strategy["fail-fast"])
	}
	if failFast {
		t.Fatal("one cluster-mode browser failure cancels the other engines")
	}
	if !strings.Contains(browser.If, "cluster-mode-auth") {
		t.Fatal("cluster-mode browser validation is not selected with its capability group")
	}
	if !containsRun(browser.Steps, "test-cluster-mode-browser") {
		t.Fatal("cluster-mode browser job does not run the dedicated browser target")
	}
	run := requireRunStep(t, browser, "test-cluster-mode-browser")
	if !strings.Contains(run.Run, "matrix.browser") {
		t.Fatal("cluster-mode browser target does not receive the selected engine")
	}
	requireClusterModeFailureHandling(t, browser)
	auth := requireJob(t, workflow, "cluster-mode-auth")
	requireClusterModeFailureHandling(t, auth)
}

func TestReleaseImagePublication(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	image := requireJob(t, workflow, "image")
	if image.Permissions["packages"] != "write" {
		t.Fatal("image job cannot publish packages")
	}
	push := requireStep(t, image, "push")
	pushed, ok := push.With["push"].(bool)
	if !ok || !pushed {
		t.Fatal("image build does not push")
	}
	if push.With["platforms"] != "linux/amd64,linux/arm64" {
		t.Fatalf("image platforms are %v", push.With["platforms"])
	}
	tags, ok := push.With["tags"].(string)
	if !ok {
		t.Fatal("image tags are not a string")
	}
	if !strings.Contains(tags, "needs.version.outputs.version") {
		t.Fatal("image tags do not use the release version")
	}
}

func TestReleaseChartPublication(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	chart := requireJob(t, workflow, "chart")
	if !contains(chart.Needs, "image") {
		t.Fatal("chart job does not wait for the image")
	}
	if chart.Permissions["packages"] != "write" {
		t.Fatal("chart job cannot publish packages")
	}
	commands := []string{"helm package", "helm push"}
	for _, command := range commands {
		if !containsRun(chart.Steps, command) {
			t.Errorf("chart job does not run %q", command)
		}
	}
	publish := requireJob(t, workflow, "publish")
	if !contains(publish.Needs, "chart") {
		t.Fatal("publish job does not wait for the chart")
	}
}

func requireRunStep(t *testing.T, job workflowJob, command string) workflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if strings.Contains(step.Run, command) {
			return step
		}
	}
	t.Fatalf("job does not run %q", command)
	return workflowStep{}
}

func requireClusterModeFailureHandling(t *testing.T, job workflowJob) {
	t.Helper()
	diagnostics := requireRunStep(t, job, "cluster-mode-diagnostics")
	if diagnostics.If != "failure()" {
		t.Fatalf("cluster-mode diagnostics condition = %q, want failure()", diagnostics.If)
	}
	cleanup := requireRunStep(t, job, "cluster-mode-down")
	if cleanup.If != "always()" {
		t.Fatalf("cluster-mode cleanup condition = %q, want always()", cleanup.If)
	}
}
