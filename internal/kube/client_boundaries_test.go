package kube

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestClientRateLimitsDefaultIndependently(t *testing.T) {
	path := writeKubeconfig(t, validKubeconfig)
	cases := []struct {
		name      string
		options   Options
		wantQPS   float32
		wantBurst int
	}{
		{
			name:      "only QPS configured",
			options:   Options{Kubeconfig: path, QPS: 17},
			wantQPS:   17,
			wantBurst: clientBurst,
		},
		{
			name:      "only burst configured",
			options:   Options{Kubeconfig: path, Burst: 29},
			wantQPS:   clientQPS,
			wantBurst: 29,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle, err := LoadContext(api.ContextRef{}, tc.options)
			if err != nil {
				t.Fatalf("LoadContext: %v", err)
			}
			if bundle.Config.QPS != tc.wantQPS {
				t.Fatalf("QPS = %v, want %v", bundle.Config.QPS, tc.wantQPS)
			}
			if bundle.Config.Burst != tc.wantBurst {
				t.Fatalf("Burst = %d, want %d", bundle.Config.Burst, tc.wantBurst)
			}
		})
	}
}
