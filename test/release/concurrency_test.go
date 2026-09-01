package release_test

import (
	"strings"
	"testing"
)

func TestLongRunningChecksUsePerRefConcurrencyGroups(t *testing.T) {
	want := "${{ github.workflow }}-${{ github.event_name }}-${{ github.ref }}-${{ github.event.head_commit.id }}"
	for _, name := range []string{
		".github/workflows/go-fuzz.yaml",
		".github/workflows/go-mutation.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			workflow := readYAML[workflowFile](t, name)
			if workflow.Concurrency.Group != want {
				t.Fatalf("concurrency group = %q, want %q", workflow.Concurrency.Group, want)
			}
		})
	}
}

func TestE2EUsesAConcurrencyGroupForEachPullRequest(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/e2e.yaml")
	want := "e2e-${{ github.event.pull_request.number || inputs.pull_request || github.ref }}-${{ github.event.head_commit.id }}"
	if workflow.Concurrency.Group != want {
		t.Fatalf("concurrency group = %q, want %q", workflow.Concurrency.Group, want)
	}
}

func TestMainPushValidationCannotBeCancelledByLaterPush(t *testing.T) {
	want := workflowScalar("${{ github.event_name != 'push' }}")
	pushID := "${{ github.event.head_commit.id }}"
	for _, name := range []string{
		".github/workflows/badges.yaml",
		".github/workflows/codeql.yaml",
		".github/workflows/commits.yaml",
		".github/workflows/e2e.yaml",
		".github/workflows/frontend.yaml",
		".github/workflows/go-fuzz.yaml",
		".github/workflows/go-mutation.yaml",
		".github/workflows/go.yaml",
		".github/workflows/integration.yaml",
		".github/workflows/repo.yaml",
		".github/workflows/windows.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			workflow := readYAML[workflowFile](t, name)
			if workflow.Concurrency.CancelInProgress != want {
				t.Fatalf("cancel-in-progress = %q, want %q", workflow.Concurrency.CancelInProgress, want)
			}
			if !strings.Contains(workflow.Concurrency.Group, pushID) {
				t.Fatalf("concurrency group %q does not distinguish main pushes", workflow.Concurrency.Group)
			}
		})
	}
}

func TestReleaseWorkflowsNeverCancelInProgress(t *testing.T) {
	want := workflowScalar("false")
	for _, name := range []string{
		".github/workflows/release-artifacts.yaml",
		".github/workflows/release-please.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			workflow := readYAML[workflowFile](t, name)
			if workflow.Concurrency.CancelInProgress != want {
				t.Fatalf("cancel-in-progress = %q, want %q", workflow.Concurrency.CancelInProgress, want)
			}
		})
	}
}
