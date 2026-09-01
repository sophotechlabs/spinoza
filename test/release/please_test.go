package release_test

import (
	"strings"
	"testing"
)

func TestReleasePleaseDispatchesPullRequestValidation(t *testing.T) {
	workflow := readYAML[workflow](t, ".github/workflows/release-please.yaml")
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
