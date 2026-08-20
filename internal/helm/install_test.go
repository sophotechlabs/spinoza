package helm

import (
	"errors"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func installRequest() InstallRequest {
	return InstallRequest{
		Namespace: "demo",
		Name:      "podinfo",
		Chart:     "podinfo",
		Version:   "6.10.0",
		RepoURL:   "https://charts.example.com",
		Values:    "replicaCount: 2\n",
	}
}

func installer(t *testing.T, runner Runner, objs ...runtime.Object) *Service {
	t.Helper()
	return NewService(k8sfake.NewClientset(objs...), nil, runner, nil, nil, api.ContextRef{Name: "kind-spinoza"})
}

func namespaceObject(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func TestAnInstallNamesTheChartTheRepoAndTheVersion(t *testing.T) {
	runner := &stubRunner{out: "NAME: podinfo"}
	svc := installer(t, runner, namespaceObject("demo"))

	result, err := svc.Install(t.Context(), installRequest())
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	args := runner.args[0]
	if args[0] != "install" || args[1] != "podinfo" || args[2] != "podinfo" {
		t.Fatalf("args = %v", args)
	}
	if !hasPair(args, "--repo", "https://charts.example.com") {
		t.Fatalf("args = %v, want the repo passed", args)
	}
	if !hasPair(args, "--version", "6.10.0") {
		t.Fatalf("args = %v, want the version passed", args)
	}
	if !hasPair(args, "--namespace", "demo") {
		t.Fatalf("args = %v, want the namespace passed", args)
	}
	if slices.Contains(args, "--create-namespace") {
		t.Fatalf("args = %v, want no --create-namespace when it was not asked for", args)
	}
	if result.Action != ActionInstall || result.Message != "NAME: podinfo" {
		t.Fatalf("result = %+v", result)
	}
}

func TestAnInstallCreatesTheNamespaceWhenAsked(t *testing.T) {
	runner := &stubRunner{}
	svc := installer(t, runner)
	req := installRequest()
	req.CreateNamespace = true

	_, err := svc.Install(t.Context(), req)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if !slices.Contains(runner.args[0], "--create-namespace") {
		t.Fatalf("args = %v, want --create-namespace", runner.args[0])
	}
}

func TestAnInstallFromAnOCIRegistryCarriesTheRefNotTheRepoFlag(t *testing.T) {
	runner := &stubRunner{}
	svc := installer(t, runner, namespaceObject("demo"))
	req := installRequest()
	req.RepoURL = "oci://registry.example.com/charts"
	req.OCI = true

	_, err := svc.Install(t.Context(), req)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	args := runner.args[0]
	if args[2] != "oci://registry.example.com/charts/podinfo" {
		t.Fatalf("chart ref = %q", args[2])
	}
	if slices.Contains(args, "--repo") {
		t.Fatalf("args = %v, want no --repo for an oci chart", args)
	}
}

func TestAPreviewRendersOnTheServerWhenTheNamespaceIsThere(t *testing.T) {
	runner := &stubRunner{out: `{"manifest":"kind: Service\n"}`}
	svc := installer(t, runner, namespaceObject("demo"))
	req := installRequest()
	req.DryRun = true

	result, err := svc.Install(t.Context(), req)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if !slices.Contains(runner.args[0], "--dry-run=server") {
		t.Fatalf("args = %v, want a server dry run", runner.args[0])
	}
	if !result.DryRun || result.Manifest != "kind: Service\n" {
		t.Fatalf("result = %+v", result)
	}
	if !strings.HasPrefix(result.Message, "server render") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestAPreviewRendersLocallyWhenTheNamespaceIsNotThereYet(t *testing.T) {
	runner := &stubRunner{out: `{"manifest":"kind: Service\n"}`}
	svc := installer(t, runner)
	req := installRequest()
	req.DryRun = true
	req.CreateNamespace = true

	result, err := svc.Install(t.Context(), req)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if !slices.Contains(runner.args[0], "--dry-run=client") {
		t.Fatalf("args = %v, want a client dry run", runner.args[0])
	}
	if !strings.HasPrefix(result.Message, "local render") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestAPreviewThatIsNotJSONIsReported(t *testing.T) {
	runner := &stubRunner{out: "not json"}
	svc := installer(t, runner, namespaceObject("demo"))
	req := installRequest()
	req.DryRun = true

	_, err := svc.Install(t.Context(), req)

	if err == nil {
		t.Fatal("unreadable dry-run output was accepted")
	}
}

func TestAnInstallRefusesWhatItCannotRun(t *testing.T) {
	cases := map[string]func(req *InstallRequest){
		"a namespace that is not a kubernetes name": func(req *InstallRequest) { req.Namespace = "Demo" },
		"a release that is not a kubernetes name":   func(req *InstallRequest) { req.Name = "Podinfo" },
		"a chart that is not a chart name":          func(req *InstallRequest) { req.Chart = "../etc/passwd" },
		"a version that is not semantic":            func(req *InstallRequest) { req.Version = "latest" },
		"a repository on this machine":              func(req *InstallRequest) { req.RepoURL = "http://127.0.0.1:8080" },
		"values that are not a yaml mapping":        func(req *InstallRequest) { req.Values = "[1, 2" },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			runner := &stubRunner{}
			svc := installer(t, runner)
			req := installRequest()
			breakIt(&req)

			_, err := svc.Install(t.Context(), req)

			if err == nil {
				t.Fatal("the request was accepted")
			}
			if len(runner.args) != 0 {
				t.Fatalf("helm was run anyway: %v", runner.args)
			}
		})
	}
}

func TestAnInstallReportsWhatHelmSaid(t *testing.T) {
	runner := &stubRunner{err: errors.New("chart not found")}
	svc := installer(t, runner, namespaceObject("demo"))

	_, err := svc.Install(t.Context(), installRequest())

	if err == nil {
		t.Fatal("a failed install reported success")
	}
}

func TestAnInstallWithoutARunnerIsRefused(t *testing.T) {
	svc := NewService(k8sfake.NewClientset(), nil, nil, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	_, err := svc.Install(t.Context(), installRequest())

	if err == nil {
		t.Fatal("an install without a runner reported success")
	}
}

func hasPair(args []string, flag, value string) bool {
	for i, arg := range args {
		if arg != flag {
			continue
		}
		if i+1 < len(args) && args[i+1] == value {
			return true
		}
	}
	return false
}

func TestAPreviewReadsPastWhatHelmPrintsWhenItPullsFromARegistry(t *testing.T) {
	pulled := "Pulled: ghcr.io/acme/charts/podinfo:6.11.0\nDigest: sha256:455d6a04\n" +
		`{"manifest":"kind: Service\n"}`
	runner := &stubRunner{out: pulled}
	svc := installer(t, runner, namespaceObject("demo"))
	req := installRequest()
	req.RepoURL = "oci://ghcr.io/acme/charts"
	req.OCI = true
	req.DryRun = true

	result, err := svc.Install(t.Context(), req)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if result.Manifest != "kind: Service\n" {
		t.Fatalf("manifest = %q, want the render helm printed after the pull notice", result.Manifest)
	}
}

func TestAPreviewWithNothingThatLooksLikeJSONIsStillReported(t *testing.T) {
	runner := &stubRunner{out: "Pulled: ghcr.io/acme/charts/podinfo:6.11.0"}
	svc := installer(t, runner, namespaceObject("demo"))
	req := installRequest()
	req.DryRun = true

	_, err := svc.Install(t.Context(), req)

	if err == nil {
		t.Fatal("output with no render at all was accepted")
	}
}
