package helm

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type valuesReadingRunner struct {
	stubRunner

	valuesPath string
	valuesSeen string
	valuesMode fs.FileMode
}

func (v *valuesReadingRunner) Run(ctx context.Context, args, env []string) (string, error) {
	for i, arg := range args {
		if arg != "--values" {
			continue
		}
		v.valuesPath = args[i+1]
		body, err := os.ReadFile(args[i+1])
		if err != nil {
			return "", err
		}
		v.valuesSeen = string(body)
		info, statErr := os.Stat(args[i+1])
		if statErr != nil {
			return "", statErr
		}
		v.valuesMode = info.Mode()
	}
	return v.stubRunner.Run(ctx, args, env)
}

func upgradeSpec() UpgradeRequest {
	return UpgradeRequest{
		Namespace: "demo",
		Name:      "podinfo",
		Chart:     "podinfo",
		Version:   "6.10.0",
		RepoURL:   "https://charts.example.com",
		Values:    "replicaCount: 2\n",
	}
}

func TestUpgradeRunsHelmAgainstTheChosenRepo(t *testing.T) {
	runner := &valuesReadingRunner{}
	service := acting(t, runner, helmSecret(sampleRelease()))

	got, err := service.Upgrade(context.Background(), upgradeSpec())
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	args := strings.Join(runner.args[0], " ")
	//nolint:dupword // the release and the chart share a name
	want := "upgrade podinfo podinfo --namespace demo --version 6.10.0" +
		" --repo https://charts.example.com --values " + runner.valuesPath +
		" --kube-context kind-spinoza"
	if args != want {
		t.Fatalf("args = %q, want %q", args, want)
	}
	if got.Message != "upgraded podinfo to podinfo 6.10.0" {
		t.Fatalf("message = %q, want a plain summary when helm says nothing", got.Message)
	}
	if got.DryRun {
		t.Fatal("a real upgrade was marked as a dry run")
	}
}

func TestUpgradeWritesTheValuesToAPrivateFileAndRemovesIt(t *testing.T) {
	runner := &valuesReadingRunner{}
	service := acting(t, runner, helmSecret(sampleRelease()))

	_, err := service.Upgrade(context.Background(), upgradeSpec())
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	if runner.valuesSeen != "replicaCount: 2\n" {
		t.Fatalf("values = %q, want them written for helm to read", runner.valuesSeen)
	}
	if runner.valuesMode.Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", runner.valuesMode.Perm())
	}
	_, statErr := os.Stat(runner.valuesPath)
	if !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("stat after upgrade = %v, want the values file removed", statErr)
	}
}

func TestUpgradePassesAnEmptyValuesFileForNoOverrides(t *testing.T) {
	runner := &valuesReadingRunner{}
	service := acting(t, runner, helmSecret(sampleRelease()))
	spec := upgradeSpec()
	spec.Values = ""

	_, err := service.Upgrade(context.Background(), spec)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	if runner.valuesPath == "" {
		t.Fatal("no values file was passed; chart defaults must come from an explicit empty file")
	}
	if runner.valuesSeen != "" {
		t.Fatalf("values = %q, want an empty file", runner.valuesSeen)
	}
}

func TestUpgradeBuildsAnOCIChartReference(t *testing.T) {
	runner := &valuesReadingRunner{}
	service := acting(t, runner, helmSecret(sampleRelease()))
	spec := upgradeSpec()
	spec.RepoURL = "oci://ghcr.io/acme/charts/"
	spec.OCI = true

	_, err := service.Upgrade(context.Background(), spec)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	args := strings.Join(runner.args[0], " ")
	want := "upgrade podinfo oci://ghcr.io/acme/charts/podinfo --namespace demo --version 6.10.0" +
		" --values " + runner.valuesPath + " --kube-context kind-spinoza"
	if args != want {
		t.Fatalf("args = %q, want %q", args, want)
	}
}

func TestUpgradeDryRunAsksForAServerRender(t *testing.T) {
	runner := &valuesReadingRunner{}
	runner.out = `{"manifest":"kind: ConfigMap\n"}`
	service := acting(t, runner, helmSecret(sampleRelease()))
	spec := upgradeSpec()
	spec.DryRun = true

	got, err := service.Upgrade(context.Background(), spec)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}

	args := strings.Join(runner.args[0], " ")
	if !strings.Contains(args, "--dry-run=server --output json") {
		t.Fatalf("args = %q, want the server dry run asked for", args)
	}
	if !got.DryRun {
		t.Fatal("the result was not marked as a dry run")
	}
	if got.Manifest != "kind: ConfigMap\n" {
		t.Fatalf("manifest = %q, want the rendered manifest", got.Manifest)
	}
	if got.Message != "server render of podinfo 6.10.0" {
		t.Fatalf("message = %q", got.Message)
	}
}

func TestUpgradeDryRunReportsUnreadableHelmOutput(t *testing.T) {
	runner := &valuesReadingRunner{}
	runner.out = "not json"
	service := acting(t, runner, helmSecret(sampleRelease()))
	spec := upgradeSpec()
	spec.DryRun = true

	_, err := service.Upgrade(context.Background(), spec)

	if err == nil {
		t.Fatal("expected unreadable dry-run output to surface")
	}
	if !strings.Contains(err.Error(), "dry-run output") {
		t.Fatalf("error = %v, want it to name the dry-run output", err)
	}
}

func TestUpgradeKeepsHelmsOwnMessage(t *testing.T) {
	runner := &valuesReadingRunner{}
	runner.out = `Release "podinfo" has been upgraded. Happy Helming!`
	service := acting(t, runner, helmSecret(sampleRelease()))

	got, err := service.Upgrade(context.Background(), upgradeSpec())
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	if !strings.Contains(got.Message, "Happy Helming") {
		t.Fatalf("message = %q, want helm's own output", got.Message)
	}
}

func TestUpgradeTellsHelmWhichDriverHoldsTheRelease(t *testing.T) {
	runner := &valuesReadingRunner{}
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1.podinfo.v1",
			Namespace: "demo",
			Labels:    map[string]string{"owner": "helm", "name": "podinfo", "version": "1"},
		},
		Data: map[string]string{releaseKey: "rubbish"},
	}
	client := k8sfake.NewClientset(entry)
	service := NewService(
		client,
		mirrorMeta(client),
		runner,
		nil,
		actionRepositories(),
		api.ContextRef{Name: "kind-spinoza"},
	)

	_, err := service.Upgrade(context.Background(), upgradeSpec())
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	env := strings.Join(runner.envs[0], " ")
	if env != "HELM_DRIVER=configmap" {
		t.Fatalf("env = %q, want the configmap driver named", env)
	}
}

func TestUpgradeRefusesAReleaseItCannotFind(t *testing.T) {
	runner := &valuesReadingRunner{}
	service := acting(t, runner)

	_, err := service.Upgrade(context.Background(), upgradeSpec())

	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
	if len(runner.args) != 0 {
		t.Fatal("helm ran for a release that does not exist")
	}
}

func TestUpgradeValidatesItsInputBeforeRunningHelm(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(spec *UpgradeRequest)
	}{
		{"flag-shaped release", func(spec *UpgradeRequest) { spec.Name = "--kubeconfig=/etc/shadow" }},
		{"flag-shaped chart", func(spec *UpgradeRequest) { spec.Chart = "--post-renderer" }},
		{"version that is not semver", func(spec *UpgradeRequest) { spec.Version = "latest" }},
		{"repo with a forbidden scheme", func(spec *UpgradeRequest) { spec.RepoURL = "ftp://charts.example.com" }},
		{"repo with no host", func(spec *UpgradeRequest) { spec.RepoURL = "https://" }},
		{"values that are not a mapping", func(spec *UpgradeRequest) { spec.Values = "- one\n- two\n" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &valuesReadingRunner{}
			service := acting(t, runner, helmSecret(sampleRelease()))
			spec := upgradeSpec()
			tc.mutate(&spec)

			_, err := service.Upgrade(context.Background(), spec)

			if err == nil {
				t.Fatal("expected the input to be refused")
			}
			if len(runner.args) != 0 {
				t.Fatal("helm ran on refused input")
			}
		})
	}
}
