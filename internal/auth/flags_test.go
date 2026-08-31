package auth

import (
	"strings"
	"testing"
)

func TestNoIdentityMeansNoImpersonationFlags(t *testing.T) {
	if got := KubectlFlags(t.Context()); got != nil {
		t.Fatalf("flags = %v, want none", got)
	}
	if got := HelmFlags(AsServer(t.Context())); got != nil {
		t.Fatalf("flags = %v, want none", got)
	}
}

func TestKubectlAndHelmAreToldWhoIsAsking(t *testing.T) {
	ctx := WithIdentity(t.Context(), Identity{
		User:   "alice@example.com",
		Groups: []string{"platform", "sre"},
	})

	kubectl := strings.Join(KubectlFlags(ctx), " ")
	if kubectl != "--as alice@example.com --as-group platform --as-group sre" {
		t.Fatalf("kubectl flags = %q", kubectl)
	}
	helm := strings.Join(HelmFlags(ctx), " ")
	if helm != "--kube-as-user alice@example.com --kube-as-group platform --kube-as-group sre" {
		t.Fatalf("helm flags = %q", helm)
	}
}
