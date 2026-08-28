package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const validKubeconfig = `apiVersion: v1
kind: Config
current-context: test-ctx
clusters:
- name: test-cluster
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: test-ctx
  context:
    cluster: test-cluster
    namespace: test-ns
    user: test-user
users:
- name: test-user
  user:
    token: test-token
`

func writeKubeconfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}

func TestLoadReturnsBundle(t *testing.T) {
	path := writeKubeconfig(t, validKubeconfig)
	t.Setenv("KUBECONFIG", path)

	bundle, err := LoadContext(api.ContextRef{}, Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if bundle.Clientset == nil {
		t.Fatal("Clientset = nil, want non-nil")
	}
	if bundle.Dynamic == nil {
		t.Fatal("Dynamic = nil, want non-nil")
	}
	if bundle.Discovery == nil {
		t.Fatal("Discovery = nil, want non-nil")
	}
	if bundle.Mapper == nil {
		t.Fatal("Mapper = nil, want non-nil")
	}
	if bundle.Ref.Name != "test-ctx" {
		t.Fatalf("Ref.Name = %q, want test-ctx", bundle.Ref.Name)
	}
	if bundle.Namespace != "test-ns" {
		t.Fatalf("Namespace = %q, want test-ns", bundle.Namespace)
	}
}

func TestLoadReturnsErrorForEmptyKubeconfig(t *testing.T) {
	path := writeKubeconfig(t, "")
	t.Setenv("KUBECONFIG", path)

	_, err := LoadContext(api.ContextRef{}, Options{})
	if err == nil {
		t.Fatal("Load returned nil error for empty kubeconfig")
	}
}

func TestLoadReturnsErrorForInvalidKubeconfig(t *testing.T) {
	path := writeKubeconfig(t, "not: [valid: yaml")
	t.Setenv("KUBECONFIG", path)

	_, err := LoadContext(api.ContextRef{}, Options{})
	if err == nil {
		t.Fatal("Load returned nil error for invalid kubeconfig")
	}
}

const twoContextKubeconfig = `apiVersion: v1
kind: Config
current-context: beta
clusters:
- name: c1
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: beta
  context:
    cluster: c1
    namespace: beta-ns
    user: u1
- name: alpha
  context:
    cluster: c1
    namespace: alpha-ns
    user: u1
users:
- name: u1
  user:
    token: t
`

const otherKubeconfig = `apiVersion: v1
kind: Config
current-context: gamma
clusters:
- name: c2
  cluster:
    server: https://127.0.0.2:6443
    insecure-skip-tls-verify: true
contexts:
- name: gamma
  context:
    cluster: c2
    user: u2
users:
- name: u2
  user:
    token: t
`

func TestReadListsEveryContextSorted(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	contexts, err := Read("")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(contexts) != 2 || contexts[0].Name != "alpha" || contexts[1].Name != "beta" {
		t.Fatalf("contexts = %v, want them sorted", contexts)
	}
	if contexts[0].Cluster != "c1" {
		t.Fatalf("cluster = %q, want the cluster the context points at", contexts[0].Cluster)
	}
	if contexts[0].Namespace != "alpha-ns" {
		t.Fatalf("namespace = %q", contexts[0].Namespace)
	}
}

func TestReadReportsAnUnreadableKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, "not: [valid: yaml"))

	_, err := Read("")

	if err == nil {
		t.Fatal("an unreadable kubeconfig was reported as having no contexts")
	}
}

func TestReadNamesAFileThatIsNotThere(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "absent"))

	if err == nil {
		t.Fatal("a kubeconfig that does not exist read as an empty one")
	}
	if !strings.Contains(err.Error(), "absent") {
		t.Fatalf("error = %v, want the path in it", err)
	}
}

func TestReadingOneFileIgnoresTheDefaultKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))
	other := writeKubeconfig(t, otherKubeconfig)

	contexts, err := Read(other)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(contexts) != 1 || contexts[0].Name != "gamma" {
		t.Fatalf("contexts = %v; a named kubeconfig must be read on its own, never merged with the default", contexts)
	}
}

func TestLoadContextTargetsTheNamedContext(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	bundle, err := LoadContext(api.ContextRef{Name: "alpha"}, Options{})
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if bundle.Ref.Name != "alpha" {
		t.Fatalf("Ref.Name = %q, want the requested one rather than the kubeconfig default", bundle.Ref.Name)
	}
	if bundle.Namespace != "alpha-ns" {
		t.Fatalf("Namespace = %q, want the namespace of the requested context", bundle.Namespace)
	}
}

func TestLoadContextReadsTheKubeconfigOnTheRef(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))
	other := writeKubeconfig(t, otherKubeconfig)

	bundle, err := LoadContext(api.ContextRef{Kubeconfig: other, Name: "gamma"}, Options{})
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if bundle.Ref.Kubeconfig != other {
		t.Fatalf("Ref.Kubeconfig = %q, want the file the context came from", bundle.Ref.Kubeconfig)
	}
	if bundle.Config.Host != "https://127.0.0.2:6443" {
		t.Fatalf("host = %q, want the server of the named kubeconfig", bundle.Config.Host)
	}
}

func TestARefWithNoKubeconfigFallsBackToTheOneSpinozaWasStartedWith(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))
	other := writeKubeconfig(t, otherKubeconfig)

	bundle, err := LoadContext(api.ContextRef{}, Options{Kubeconfig: other})
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if bundle.Ref.Name != "gamma" {
		t.Fatalf("Ref.Name = %q, want the current context of the --kubeconfig file", bundle.Ref.Name)
	}
}

func TestTheFileSpinozaWasStartedWithIsNamedOnTheRef(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))
	other := writeKubeconfig(t, otherKubeconfig)

	bundle, err := LoadContext(api.ContextRef{}, Options{Kubeconfig: other})
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if bundle.Ref.Kubeconfig != other {
		t.Fatalf("Ref.Kubeconfig = %q, want the file spinoza was started with", bundle.Ref.Kubeconfig)
	}
}

func TestAFileOnTheRefIsNotReplacedByTheOneSpinozaWasStartedWith(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, validKubeconfig))
	started := writeKubeconfig(t, twoContextKubeconfig)
	other := writeKubeconfig(t, otherKubeconfig)

	bundle, err := LoadContext(
		api.ContextRef{Kubeconfig: other, Name: "gamma"},
		Options{Kubeconfig: started},
	)
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if bundle.Ref.Kubeconfig != other {
		t.Fatalf("Ref.Kubeconfig = %q, want the file the context came from", bundle.Ref.Kubeconfig)
	}
}

func TestAStartWithNoFileNamedLeavesTheRefWithoutOne(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	bundle, err := LoadContext(api.ContextRef{}, Options{})
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if bundle.Ref.Kubeconfig != "" {
		t.Fatalf("Ref.Kubeconfig = %q, want the usual lookup rules left alone", bundle.Ref.Kubeconfig)
	}
}

func TestLoadContextRejectsAContextThatIsNotThere(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	_, err := LoadContext(api.ContextRef{Name: "gone"}, Options{})

	if err == nil {
		t.Fatal("switching to a context that does not exist reported success")
	}
}

func TestLoadContextWithNoNameKeepsTheDefault(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	bundle, err := LoadContext(api.ContextRef{}, Options{})
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if bundle.Ref.Name != "beta" {
		t.Fatalf("Ref.Name = %q, want the kubeconfig's current-context", bundle.Ref.Name)
	}
}

func TestLoadContextRaisesTheClientRateLimit(t *testing.T) {
	path := writeKubeconfig(t, validKubeconfig)
	t.Setenv("KUBECONFIG", path)

	bundle, err := LoadContext(api.ContextRef{}, Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if bundle.Config.QPS < 50 {
		t.Fatalf("QPS = %v; client-go defaults to 5, which throttles a fan-out of 150 lists to 30s", bundle.Config.QPS)
	}
	if bundle.Config.Burst < 100 {
		t.Fatalf("Burst = %d, want headroom for the catalog fan-out", bundle.Config.Burst)
	}
}

func TestLabelNamesTheFileTheContextsCameFrom(t *testing.T) {
	path := writeKubeconfig(t, validKubeconfig)

	if Label(path) != path {
		t.Fatalf("Label = %q, want the path itself", Label(path))
	}
}

func TestLabelOfTheDefaultNamesEveryFileItReads(t *testing.T) {
	first := writeKubeconfig(t, validKubeconfig)
	second := writeKubeconfig(t, otherKubeconfig)
	t.Setenv("KUBECONFIG", first+string(os.PathListSeparator)+second)

	label := Label("")

	if !strings.Contains(label, first) || !strings.Contains(label, second) {
		t.Fatalf("label = %q, want both files KUBECONFIG points at", label)
	}
}
