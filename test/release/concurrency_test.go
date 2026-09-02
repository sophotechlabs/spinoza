package release_test

import (
	"strings"
	"testing"
)

func TestLongRunningChecksUsePerRefConcurrencyGroups(t *testing.T) {
	want := "${{ github.workflow }}-${{ github.event_name }}-${{ github.ref }}-"
	for _, name := range []string{
		".github/workflows/go-fuzz.yaml",
		".github/workflows/go-mutation.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			workflow := readYAML[workflowFile](t, name)
			if !strings.HasPrefix(workflow.Concurrency.Group, want) {
				t.Fatalf("concurrency group = %q, want prefix %q", workflow.Concurrency.Group, want)
			}
		})
	}
}

func TestE2EUsesAConcurrencyGroupForEachPullRequest(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/e2e.yaml")
	want := "e2e-${{ github.event.pull_request.number || inputs.pull_request || github.ref }}-"
	if !strings.HasPrefix(workflow.Concurrency.Group, want) {
		t.Fatalf("concurrency group = %q, want prefix %q", workflow.Concurrency.Group, want)
	}
}

func TestInstallSupersedesValidationButPreservesPublishedReleaseChecks(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/install.yaml")
	wantGroup := "install-${{ inputs.version || github.event.pull_request.number || github.ref }}"
	if workflow.Concurrency.Group != wantGroup {
		t.Fatalf("concurrency group = %q, want %q", workflow.Concurrency.Group, wantGroup)
	}
	wantCancellation := workflowScalar("${{ !inputs.version }}")
	if workflow.Concurrency.CancelInProgress != wantCancellation {
		t.Fatalf("cancel-in-progress = %q, want %q", workflow.Concurrency.CancelInProgress, wantCancellation)
	}
	release := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	version, ok := requireJob(t, release, "install").With["version"].(string)
	if !ok || version != "${{ needs.version.outputs.tag }}" {
		t.Fatalf("release install version = %q, want the detected release tag", version)
	}
}

func TestOrdinaryPushesSupersedeValidationButReleaseCommitsDoNot(t *testing.T) {
	want := workflowScalar("${{ github.event_name != 'push' || !startsWith(github.event.head_commit.message, 'chore(main): release ') }}")
	release := "startsWith(github.event.head_commit.message, 'chore(main): release ')"
	pushID := "github.sha"
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
			if !strings.Contains(workflow.Concurrency.Group, release) {
				t.Fatalf("concurrency group %q does not recognize release commits", workflow.Concurrency.Group)
			}
			if !strings.Contains(workflow.Concurrency.Group, pushID) {
				t.Fatalf("concurrency group %q does not isolate the release commit", workflow.Concurrency.Group)
			}
			if !strings.Contains(workflow.Concurrency.Group, "'latest'") {
				t.Fatalf("concurrency group %q does not coalesce ordinary pushes", workflow.Concurrency.Group)
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
