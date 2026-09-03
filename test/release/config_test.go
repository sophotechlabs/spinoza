package release_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		"--helm-set auth.mode=oidc",
		"--helm-set auth.oidc.issuerURL=https://idp.example.com/realms/spinoza",
		"--helm-set auth.oidc.clientID=spinoza",
		"--helm-set auth.sessionSecret=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"bash test/trivy-helm-coverage.sh \"$report\"",
	}
	for _, flag := range want {
		if !strings.Contains(recipe, flag) {
			t.Errorf("vulns recipe does not supply %s", flag)
		}
	}
}

func TestTrivyHelmCoverageRequiresEveryRenderedResource(t *testing.T) {
	complete := `{"Results":[
		{"Target":"deploy/helm/spinoza/templates/deployment.yaml","Class":"config","Type":"helm"},
		{"Target":"deploy/helm/spinoza/templates/rbac.yaml","Class":"config","Type":"helm"},
		{"Target":"deploy/helm/spinoza/templates/secret.yaml","Class":"config","Type":"helm"},
		{"Target":"deploy/helm/spinoza/templates/service.yaml","Class":"config","Type":"helm"},
		{"Target":"deploy/helm/spinoza/templates/serviceaccount.yaml","Class":"config","Type":"helm"}
	]}`
	if output, err := runTrivyCoverage(t, complete); err != nil {
		t.Fatalf("complete report: %v: %s", err, output)
	}

	missing := strings.Replace(complete, `{"Target":"deploy/helm/spinoza/templates/secret.yaml","Class":"config","Type":"helm"},`, "", 1)
	output, err := runTrivyCoverage(t, missing)
	if err == nil {
		t.Fatal("report missing the rendered Secret passed coverage")
	}
	if !strings.Contains(output, "Trivy did not scan expected Helm target deploy/helm/spinoza/templates/secret.yaml") {
		t.Fatalf("output = %q, want the missing target", output)
	}
}

func TestTrivyHelmCoverageRejectsASkippedChart(t *testing.T) {
	report := `{"Results":[
		{"Target":"deploy/helm/spinoza/templates/deployment.yaml","Class":"config","Type":"helm"},
		{"Target":"deploy/helm/spinoza/templates/rbac.yaml","Class":"config","Type":"helm"},
		{"Target":"deploy/helm/spinoza/templates/secret.yaml","Class":"config","Type":"helm"},
		{"Target":"deploy/helm/spinoza/templates/service.yaml","Class":"config","Type":"helm"},
		{"Target":"deploy/helm/spinoza/templates/serviceaccount.yaml","Class":"config","Type":"helm"}
	],"Messages":["Skipping chart because values are invalid"]}`
	output, err := runTrivyCoverage(t, report)
	if err == nil {
		t.Fatal("report saying the chart was skipped passed coverage")
	}
	if !strings.Contains(output, "Trivy reported that it skipped a Helm chart") {
		t.Fatalf("output = %q, want the skipped-chart reason", output)
	}
}

func runTrivyCoverage(t *testing.T, report string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trivy.json")
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	script := filepath.Join(repositoryRoot(t), "test", "trivy-helm-coverage.sh")
	command := exec.CommandContext(ctx, "bash", script, path)
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestTheContainerBuildCopiesOnlyRequiredGoSources(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(data)
	if strings.Contains(dockerfile, "COPY . .") {
		t.Fatal("Dockerfile copies the complete developer context into the Go build stage")
	}
	want := []string{
		"COPY *.go ./",
		"COPY internal ./internal",
		"COPY LICENSE ./",
		"COPY --from=web /src/web/dist ./web/dist",
	}
	for _, instruction := range want {
		if !strings.Contains(dockerfile, instruction) {
			t.Errorf("Dockerfile does not contain %q", instruction)
		}
	}
}
