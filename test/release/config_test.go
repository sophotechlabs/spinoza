package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseVersionsMatch(t *testing.T) {
	manifest := readJSON[map[string]string](t, ".release-please-manifest.json")
	chart := readYAML[chart](t, "deploy/helm/spinoza/Chart.yaml")
	wails := readJSON[wails](t, "wails.json")
	released := manifest["."]
	if released == "" {
		t.Fatal("release manifest has no root version")
	}
	versions := []struct {
		name  string
		value string
	}{
		{name: "chart", value: chart.Version},
		{name: "chart app", value: chart.AppVersion},
		{name: "desktop app", value: wails.Info.ProductVersion},
	}
	for _, version := range versions {
		if version.value != released {
			t.Errorf("%s version is %q, release is %q", version.name, version.value, released)
		}
	}
}

func TestReleasePleaseCreatesTagsForDraftReleases(t *testing.T) {
	config := readJSON[releaseConfig](t, "release-please-config.json")
	pkg := requirePackage(t, config)
	if !pkg.Draft {
		t.Fatal("release-please does not create draft releases")
	}
	if !pkg.ForceTag {
		t.Fatal("release-please leaves draft releases without discoverable tags")
	}
}

func TestReleasePleaseHasNoRecoveryBoundary(t *testing.T) {
	config := readJSON[map[string]any](t, "release-please-config.json")
	if _, found := config["last-release-sha"]; found {
		t.Fatal("release-please still uses a one-time recovery boundary")
	}
}

func TestReleasePleaseUpdatesBothChartVersions(t *testing.T) {
	config := readJSON[releaseConfig](t, "release-please-config.json")
	pkg := requirePackage(t, config)
	want := map[string]bool{
		"$.version":    false,
		"$.appVersion": false,
	}
	for _, file := range pkg.ExtraFiles {
		if file.Type != "yaml" {
			continue
		}
		if file.Path != "deploy/helm/spinoza/Chart.yaml" {
			continue
		}
		if _, ok := want[file.JSONPath]; ok {
			want[file.JSONPath] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("release-please does not update Chart.yaml at %s", path)
		}
	}
}

func TestVulnerabilityScanCanRenderHelmChart(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "justfile"))
	if err != nil {
		t.Fatal(err)
	}
	after, found := strings.CutPrefix(string(data), "vulns:")
	if !found {
		_, after, found = strings.Cut(string(data), "\nvulns:")
	}
	if !found {
		t.Fatal("justfile has no vulns recipe")
	}
	recipe, _, found := strings.Cut(after, "\n\n")
	if !found {
		t.Fatal("vulns recipe has no ending boundary")
	}
	want := []string{
		"--helm-set publicURL=https://spinoza.example.com",
		"--helm-set auth.mode=none",
		"--helm-set auth.allowAnonymous=true",
	}
	for _, flag := range want {
		if !strings.Contains(recipe, flag) {
			t.Errorf("vulns recipe does not supply %s", flag)
		}
	}
}
