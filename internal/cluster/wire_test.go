package cluster

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const deadKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:1
    insecure-skip-tls-verify: true
  name: dead
contexts:
- context:
    cluster: dead
    user: nobody
  name: dead
current-context: dead
users:
- name: nobody
  user: {}
`

func TestNewStartsWithoutAClusterWhenNothingAnswers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(deadKubeconfig), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	built, err := New(ctx, Options{Kubeconfig: path})
	if err != nil {
		t.Fatalf("new: %v, want the startup failure kept for the ui instead", err)
	}
	if built.Manager() != nil {
		t.Fatal("a manager appeared with no cluster answering")
	}
	if built.Current().Name != "" {
		t.Fatalf("current = %q, want no context installed", built.Current().Name)
	}
	contexts := built.Contexts()
	if !strings.Contains(contexts.Error, "lists no resource types") {
		t.Fatalf("error = %q, want the unreachable context named", contexts.Error)
	}
	if !strings.Contains(contexts.Error, "dead") {
		t.Fatalf("error = %q, want the context name in it", contexts.Error)
	}
}

func TestNewRefusesABadPrometheusSpec(t *testing.T) {
	_, err := New(context.Background(), Options{PromSpec: "no-slash"})
	if err == nil {
		t.Fatal("expected the prom spec to be refused")
	}
	if !strings.Contains(err.Error(), "namespace/service:port") {
		t.Fatalf("err = %v, want the expected shape named", err)
	}
}
