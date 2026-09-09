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

func TestReleasePleaseOnlyCutsThePullRequest(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-please.yaml")
	job := requireJob(t, workflow, "release-please")
	if _, ok := job.Permissions["actions"]; ok {
		t.Fatal("release-please can still dispatch workflows, which doubled the cost of every commit")
	}
	release := requireStep(t, job, "release")
	if !strings.Contains(release.Uses, "googleapis/release-please-action@") {
		t.Fatal("release-please output does not come from the release action")
	}
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "gh workflow run") {
			t.Fatalf("release-please dispatches %q on every commit; validation belongs to release-artifacts", step.Name)
		}
	}
}

func TestReleaseArtifactsValidateBeforeAnythingIsBuilt(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	validators := map[string]string{
		"validate-e2e":      "./.github/workflows/e2e.yaml",
		"validate-mutation": "./.github/workflows/go-mutation.yaml",
		"validate-fuzz":     "./.github/workflows/go-fuzz.yaml",
	}
	for name, uses := range validators {
		job := requireJob(t, workflow, name)
		if job.Uses != uses {
			t.Fatalf("%s uses %q, want %q", name, job.Uses, uses)
		}
		if !strings.Contains(job.If, "needs.version.outputs.pending") {
			t.Fatalf("%s runs when no release is pending, which is every commit", name)
		}
		ref, ok := job.With["ref"].(string)
		if !ok || ref != "${{ needs.version.outputs.sha }}" {
			t.Fatalf("%s validates %v, want the release commit", name, job.With["ref"])
		}
	}
	tier, ok := requireJob(t, workflow, "validate-e2e").With["tier"].(string)
	if !ok || tier != "nightly" {
		t.Fatalf("release validation runs the e2e %v tier, want every group on every browser", requireJob(t, workflow, "validate-e2e").With["tier"])
	}
	duration, ok := requireJob(t, workflow, "validate-fuzz").With["duration"].(string)
	if !ok || duration != "10m" {
		t.Fatalf("release validation fuzzes for %v, want longer than the per-commit smoke", requireJob(t, workflow, "validate-fuzz").With["duration"])
	}
	for _, name := range []string{"dist", "image"} {
		needs := requireJob(t, workflow, name).Needs
		for validator := range validators {
			if !contains(needs, validator) {
				t.Fatalf("%s does not wait for %s, so an unvalidated release could publish", name, validator)
			}
		}
	}
}

func TestEveryCommitTierWorkflowStillGuardsTheReleaseCommit(t *testing.T) {
	tiers := readJSON[ciTiers](t, ".github/ci-tiers.json")
	seen := 0
	for name, entry := range tiers.Workflows {
		if entry.Tier != "commit" {
			continue
		}
		seen++
		path := filepath.Join(".github", "workflows", name)
		triggers := readYAML[triggerWorkflow](t, path)
		push, ok := triggers.On["push"].(map[string]any)
		if !ok {
			t.Errorf("%s is on the commit tier but does not run when a commit lands on main", path)
			continue
		}
		branches, ok := push["branches"].([]any)
		if !ok || len(branches) == 0 {
			t.Errorf("%s names no branch to run on", path)
			continue
		}
		if branches[0] != "main" {
			t.Errorf("%s runs on %v, want main, so a release commit is covered", path, branches[0])
		}
	}
	if seen == 0 {
		t.Fatal("no workflow is on the commit tier")
	}
}

func TestEveryWorkflowDeclaresACadence(t *testing.T) {
	tiers := readJSON[ciTiers](t, ".github/ci-tiers.json")
	entries, err := os.ReadDir(filepath.Join(repositoryRoot(t), ".github", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		found++
		if _, ok := tiers.Workflows[entry.Name()]; !ok {
			t.Errorf("%s has no tier in .github/ci-tiers.json", entry.Name())
		}
	}
	if found != len(tiers.Workflows) {
		t.Fatalf("%d workflow files against %d declared tiers", found, len(tiers.Workflows))
	}
}
