package helm

import (
	"context"
	"strings"
	"testing"

	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestEveryHelmFetchRefusesARepositoryThatIsNotConfigured(t *testing.T) {
	cases := []struct {
		name string
		call func(context.Context, *Service) error
	}{
		{
			name: "install",
			call: func(ctx context.Context, service *Service) error {
				req := installRequest()
				req.RepoURL = "https://attacker.example.com"
				_, err := service.Install(ctx, req)
				return err
			},
		},
		{
			name: "upgrade",
			call: func(ctx context.Context, service *Service) error {
				req := upgradeSpec()
				req.RepoURL = "https://attacker.example.com"
				_, err := service.Upgrade(ctx, req)
				return err
			},
		},
		{
			name: "chart values",
			call: func(ctx context.Context, service *Service) error {
				_, err := service.ChartValues(ctx, ValuesRequest{
					Chart:   "podinfo",
					Version: "6.10.0",
					RepoURL: "https://attacker.example.com",
				})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := k8sfake.NewClientset(helmSecret(sampleRelease()))
			runner := &stubRunner{}
			service := NewService(
				client,
				mirrorMeta(client),
				runner,
				nil,
				entriesOf("https://charts.example.com"),
				api.ContextRef{Name: "kind-spinoza"},
			)

			err := tc.call(t.Context(), service)

			if err == nil || !strings.Contains(err.Error(), "not configured") {
				t.Fatalf("error = %v, want the unconfigured repository refused", err)
			}
			if len(runner.args) != 0 {
				t.Fatalf("helm ran with an unconfigured repository: %v", runner.args)
			}
		})
	}
}

func TestAConfiguredRepositoryCannotBeChangedFromHTTPToOCI(t *testing.T) {
	runner := &stubRunner{}
	service := NewService(
		k8sfake.NewClientset(),
		nil,
		runner,
		nil,
		entriesOf("https://charts.example.com"),
		api.ContextRef{Name: "kind-spinoza"},
	)

	_, err := service.ChartValues(t.Context(), ValuesRequest{
		Chart:   "podinfo",
		Version: "6.10.0",
		RepoURL: "https://charts.example.com",
		OCI:     true,
	})

	if err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("error = %v, want the protocol mismatch refused", err)
	}
	if len(runner.args) != 0 {
		t.Fatalf("helm ran with a changed repository protocol: %v", runner.args)
	}
}
