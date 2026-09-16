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

func TestReleaseArtifactsOnlyBuildAndPublish(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	for name, job := range workflow.Jobs {
		if job.Uses != "" {
			t.Errorf("release artifacts calls %s through the %s job; a release waits on nothing but its own artifacts", job.Uses, name)
		}
	}
	want := map[string]bool{
		"version":       true,
		"dist":          true,
		"image":         true,
		"chart":         true,
		"desktop":       true,
		"desktop-linux": true,
		"publish":       true,
	}
	for name := range workflow.Jobs {
		if !want[name] {
			t.Errorf("release artifacts runs %s, which does not build or publish an artifact", name)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("release artifacts no longer has: %v", want)
	}
	for _, name := range []string{"dist", "image"} {
		needs := requireJob(t, workflow, name).Needs
		if len(needs) != 1 || needs[0] != "version" {
			t.Fatalf("%s needs %v, want only the version job", name, needs)
		}
	}
}

func TestEveryReleaseJobIsBounded(t *testing.T) {
	workflow := readYAML[workflowFile](t, ".github/workflows/release-artifacts.yaml")
	for name, job := range workflow.Jobs {
		minutes, ok := job.TimeoutMinutes.(int)
		if !ok {
			t.Errorf("%s job has no timeout, so a stuck release hangs for six hours", name)
			continue
		}
		if minutes <= 0 || minutes > 45 {
			t.Errorf("%s job timeout = %d minutes, want a bound between 1 and 45", name, minutes)
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
