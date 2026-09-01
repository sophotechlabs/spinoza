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
			if !workflow.Concurrency.CancelInProgress {
				t.Fatal("superseded workflow runs are not cancelled")
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
	if !workflow.Concurrency.CancelInProgress {
		t.Fatal("superseded workflow runs are not cancelled")
	}
}

func TestLongCampaignsRunOnceAndSkipReleaseOnlyPullRequests(t *testing.T) {
	releaseFiles := []string{
		".release-please-manifest.json",
		"CHANGELOG.md",
		"deploy/helm/spinoza/Chart.yaml",
		"wails.json",
	}
	for _, name := range []string{
		".github/workflows/go-fuzz.yaml",
		".github/workflows/go-mutation.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			workflow := readYAML[workflowFile](t, name)
			if len(workflow.On.Push.Branches) != 1 || workflow.On.Push.Branches[0] != "main" {
				t.Fatalf("push branches = %v, want only main", workflow.On.Push.Branches)
			}
			for _, path := range releaseFiles {
				if !contains(workflow.On.PullRequest.PathsIgnore, path) {
					t.Fatalf("release-only pull requests do not ignore %s", path)
				}
			}
		})
	}
}
