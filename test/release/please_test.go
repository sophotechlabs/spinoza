package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type triggerWorkflow struct {
	On map[string]any `yaml:"on"`
}

func TestReleasePleaseDispatchesPullRequestValidation(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-please.yaml")
	job := requireJob(t, workflow, "release-please")
	if job.Permissions["actions"] != "write" {
		t.Fatal("release-please cannot dispatch validation")
	}
	release := requireStep(t, job, "release")
	if !strings.Contains(release.Uses, "googleapis/release-please-action@") {
		t.Fatal("release-please output does not come from the release action")
	}
	dispatch := requireNamedStep(t, job, "Validate the release pull request")
	if !strings.Contains(dispatch.If, "steps.release.outputs.prs_created") {
		t.Fatal("release validation is not gated on a created pull request")
	}
	if dispatch.Env["GH_REPO"] != "${{ github.repository }}" {
		t.Fatal("release validation does not identify its GitHub repository")
	}
	if !strings.Contains(dispatch.Run, "gh workflow run e2e.yaml") {
		t.Fatal("release validation does not dispatch the end-to-end workflow")
	}
	if !strings.Contains(dispatch.Run, "pull_request=") {
		t.Fatal("release validation does not identify the release pull request")
	}
}

func TestReleasePleaseDispatchesEveryPullRequestWorkflow(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-please.yaml")
	dispatch := requireNamedStep(t, requireJob(t, workflow, "release-please"), "Validate the release pull request")
	entries, err := os.ReadDir(filepath.Join(repositoryRoot(t), ".github", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(".github", "workflows", entry.Name())
		triggers := readYAML[triggerWorkflow](t, path)
		if _, ok := triggers.On["pull_request"]; !ok {
			continue
		}
		if _, ok := triggers.On["workflow_dispatch"]; !ok {
			t.Errorf("%s validates pull requests but cannot be dispatched", path)
			continue
		}
		if !strings.Contains(dispatch.Run, entry.Name()) {
			t.Errorf("release validation does not dispatch %s", path)
		}
	}
}
