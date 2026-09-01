package release_test

import (
	"regexp"
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

func TestReleasePleaseLeavesDraftTagCreationToPublication(t *testing.T) {
	config := readJSON[releaseConfig](t, "release-please-config.json")
	pkg := requirePackage(t, config)
	if pkg.ForceTag {
		t.Fatal("release-please force-creates a tag before the draft can be published")
	}
}

func TestConfiguredRecoveryBoundaryIsACommitSHA(t *testing.T) {
	config := readJSON[releaseConfig](t, "release-please-config.json")
	if config.LastReleaseSHA == "" {
		return
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(config.LastReleaseSHA) {
		t.Fatalf("release recovery boundary %q is not a full commit SHA", config.LastReleaseSHA)
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
