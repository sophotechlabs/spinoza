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
