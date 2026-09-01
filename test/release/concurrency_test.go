package release_test

import "testing"

func TestLongRunningChecksCancelSupersededCommits(t *testing.T) {
	want := "${{ github.workflow }}-${{ github.event_name }}-${{ github.ref }}"
	for _, name := range []string{
		".github/workflows/go-fuzz.yaml",
		".github/workflows/go-mutation.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			workflow := readYAML[workflowFile](t, name)
			if workflow.Concurrency.Group != want {
				t.Fatalf("concurrency group = %q, want %q", workflow.Concurrency.Group, want)
			}
			if workflow.Concurrency.CancelInProgress != "true" {
				t.Fatalf("cancel-in-progress = %q, want true", workflow.Concurrency.CancelInProgress)
			}
		})
	}
}

func TestE2ECancelsSupersededCommitsAndPullRequests(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/e2e.yaml")
	want := "${{ github.workflow }}-${{ github.event_name }}-${{ github.event.pull_request.number || inputs.pull_request || github.ref }}"
	if workflow.Concurrency.Group != want {
		t.Fatalf("concurrency group = %q, want %q", workflow.Concurrency.Group, want)
	}
	if workflow.Concurrency.CancelInProgress != "true" {
		t.Fatalf("cancel-in-progress = %q, want true", workflow.Concurrency.CancelInProgress)
	}
}

func TestGoValidationFinishesOnMainAndCancelsSupersededPullRequests(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/go.yaml")
	want := workflowScalar("${{ github.event_name == 'pull_request' }}")
	if workflow.Concurrency.CancelInProgress != want {
		t.Fatalf("cancel-in-progress = %q, want %q", workflow.Concurrency.CancelInProgress, want)
	}
}
