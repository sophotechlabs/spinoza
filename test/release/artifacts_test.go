package release_test

import (
	"strings"
	"testing"
)

func TestReleaseArtifactsRunOnEveryMainPush(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	if len(workflow.On.Push.Paths) != 0 {
		t.Fatalf("release artifact pushes are restricted to %v", workflow.On.Push.Paths)
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
}

func TestReleaseArtifactBuildsAreGatedOnPendingWork(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	jobs := []string{"dist", "image", "chart", "desktop", "desktop-linux", "publish", "install"}
	for _, name := range jobs {
		job := requireJob(t, workflow, name)
		if !strings.Contains(job.If, "pending") {
			t.Errorf("%s job is not gated on pending release work", name)
		}
	}
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
