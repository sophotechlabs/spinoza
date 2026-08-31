package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNothingIsWrittenWithNowhereToWriteIt(t *testing.T) {
	_, err := WriteInClusterKubeconfig("")

	if err == nil {
		t.Fatal("a kubeconfig was written to nowhere")
	}
	if !strings.Contains(err.Error(), "no directory") {
		t.Fatalf("error = %q, want it to say why", err.Error())
	}
}

func TestOutsideAClusterThereIsNoInClusterKubeconfig(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	if InCluster() {
		t.Fatal("a laptop was mistaken for a pod")
	}
	_, err := WriteInClusterKubeconfig(t.TempDir())
	if err == nil {
		t.Fatal("an in-cluster kubeconfig was written outside a cluster")
	}
}

func TestTheKubeconfigHelmAndKubectlReadIsTheOnDiskShape(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte("a-token"), 0o600); err != nil {
		t.Fatalf("writing the token: %v", err)
	}

	path, err := writeToolKubeconfig(dir, "https://10.96.0.1:443")
	if err != nil {
		t.Fatalf("writing the kubeconfig: %v", err)
	}

	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading it back: %v", readErr)
	}
	text := string(body)
	for _, want := range []string{
		"clusters:\n- cluster:",
		"server: https://10.96.0.1:443",
		"certificate-authority: " + caPath,
		"tokenFile: " + tokenPath,
		"current-context: " + inClusterName,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("the kubeconfig is missing %q:\n%s", want, text)
		}
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600; it names the pod's token file", info.Mode().Perm())
	}
}
