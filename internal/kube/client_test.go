package kube

import (
	"os"
	"path/filepath"
	"testing"
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

	bundle, err := Load()
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
	if bundle.Context != "test-ctx" {
		t.Fatalf("Context = %q, want test-ctx", bundle.Context)
	}
	if bundle.Namespace != "test-ns" {
		t.Fatalf("Namespace = %q, want test-ns", bundle.Namespace)
	}
}

func TestLoadReturnsErrorForEmptyKubeconfig(t *testing.T) {
	path := writeKubeconfig(t, "")
	t.Setenv("KUBECONFIG", path)

	_, err := Load()
	if err == nil {
		t.Fatal("Load returned nil error for empty kubeconfig")
	}
}

func TestLoadReturnsErrorForInvalidKubeconfig(t *testing.T) {
	path := writeKubeconfig(t, "not: [valid: yaml")
	t.Setenv("KUBECONFIG", path)

	_, err := Load()
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

func TestContextsListsEveryContextSorted(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	names, current, err := Contexts()
	if err != nil {
		t.Fatalf("Contexts: %v", err)
	}

	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("names = %v, want them sorted", names)
	}
	if current != "beta" {
		t.Fatalf("current = %q", current)
	}
}

func TestContextsReportsAnUnreadableKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, "not: [valid: yaml"))

	_, _, err := Contexts()

	if err == nil {
		t.Fatal("an unreadable kubeconfig was reported as having no contexts")
	}
}

func TestLoadContextTargetsTheNamedContext(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	bundle, err := LoadContext("alpha")
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if bundle.Context != "alpha" {
		t.Fatalf("Context = %q, want the requested one rather than the kubeconfig default", bundle.Context)
	}
	if bundle.Namespace != "alpha-ns" {
		t.Fatalf("Namespace = %q, want the namespace of the requested context", bundle.Namespace)
	}
}

func TestLoadContextRejectsAContextThatIsNotThere(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	_, err := LoadContext("gone")

	if err == nil {
		t.Fatal("switching to a context that does not exist reported success")
	}
}

func TestLoadContextWithNoNameKeepsTheDefault(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	bundle, err := LoadContext("")
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if bundle.Context != "beta" {
		t.Fatalf("Context = %q, want the kubeconfig's current-context", bundle.Context)
	}
}
