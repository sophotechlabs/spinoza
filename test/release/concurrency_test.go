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

func TestMutationTestingIsBoundedAndSplitIntoPackageShards(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/go-mutation.yaml")
	mutation := requireJob(t, workflow, "mutation")
	if mutation.TimeoutMinutes != 60 {
		t.Fatalf("mutation timeout = %d minutes, want 60", mutation.TimeoutMinutes)
	}
	matrix, ok := mutation.Strategy["matrix"].(map[string]any)
	if !ok {
		t.Fatalf("mutation matrix = %#v, want a mapping", mutation.Strategy["matrix"])
	}
	shards, ok := matrix["shard"].([]any)
	if !ok {
		t.Fatalf("mutation shards = %#v, want a list", matrix["shard"])
	}
	if len(shards) != 23 {
		t.Fatalf("mutation shard count = %d, want 23", len(shards))
	}
	wantShards := map[string]bool{
		"root-default":  true,
		"root-desktop":  true,
		"cmd":           true,
		"internal-a-d":  true,
		"checks-a-e":    true,
		"checks-f-j":    true,
		"checks-k-o":    true,
		"checks-p-r":    true,
		"checks-s-t":    true,
		"checks-u-z":    true,
		"internal-e-l":  true,
		"internal-m-r":  true,
		"resources-a-f": true,
		"resources-g-l": true,
		"resources-m-r": true,
		"resources-s-z": true,
		"server-a-c":    true,
		"server-d-e":    true,
		"server-f":      true,
		"server-g-l":    true,
		"server-m-r":    true,
		"server-s-z":    true,
		"internal-s-z":  true,
	}
	for _, raw := range shards {
		shard, ok := raw.(string)
		if !ok {
			t.Fatalf("mutation shard = %#v, want a string", raw)
		}
		if !wantShards[shard] {
			t.Fatalf("unexpected mutation shard %q", shard)
		}
		delete(wantShards, shard)
	}
	if len(wantShards) != 0 {
		t.Fatalf("missing mutation shards: %v", wantShards)
	}
	total := requireJob(t, workflow, "mutation-total")
	if total.TimeoutMinutes != 10 {
		t.Fatalf("mutation total timeout = %d minutes, want 10", total.TimeoutMinutes)
	}
	if len(total.Needs) != 1 || total.Needs[0] != "mutation" {
		t.Fatalf("mutation total needs = %v, want mutation", total.Needs)
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
