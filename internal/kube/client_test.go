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

func TestLoadReturnsClientsetContextAndNamespace(t *testing.T) {
	path := writeKubeconfig(t, validKubeconfig)
	t.Setenv("KUBECONFIG", path)

	cs, contextName, namespace, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cs == nil {
		t.Fatal("clientset = nil, want non-nil")
	}
	if contextName != "test-ctx" {
		t.Fatalf("contextName = %q, want test-ctx", contextName)
	}
	if namespace != "test-ns" {
		t.Fatalf("namespace = %q, want test-ns", namespace)
	}
}

func TestLoadReturnsErrorForEmptyKubeconfig(t *testing.T) {
	path := writeKubeconfig(t, "")
	t.Setenv("KUBECONFIG", path)

	_, _, _, err := Load()
	if err == nil {
		t.Fatal("Load returned nil error for empty kubeconfig")
	}
}
