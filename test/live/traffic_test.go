package live

import (
	"context"
	"os"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/traffic"
)

const liveTraffic = "SPINOZA_LIVE_TRAFFIC"

func TestTheLiveClusterAnswersATrafficGraphFromHubble(t *testing.T) {
	if os.Getenv(liveTraffic) != "1" {
		t.Skip(liveTraffic + " is not set")
	}
	loading := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loading,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	client := prom.NewClient(clientset, prom.Target{})
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	target, err := client.Target(ctx)
	if err != nil {
		t.Fatalf("find prometheus: %v", err)
	}
	reader := traffic.New(client)
	now := time.Now()
	support := reader.Support(ctx, now)
	if !support.Available {
		t.Fatalf("traffic unavailable: %s", support.Reason)
	}
	if support.Source != "Cilium Hubble" {
		t.Fatalf("source = %q, want Cilium Hubble", support.Source)
	}
	graph := reader.Graph(ctx, now)
	if graph.Error != "" {
		t.Fatalf("graph: %s", graph.Error)
	}
	if len(graph.Nodes) < 2 {
		t.Fatalf("nodes = %d, want traffic between at least two workloads", len(graph.Nodes))
	}
	if len(graph.Edges) == 0 {
		t.Fatal("the Hubble metrics produced no workload edge")
	}
	t.Logf(
		"prometheus=%s source=%s nodes=%d edges=%d folded=%t workloads=%d",
		target,
		graph.Source,
		len(graph.Nodes),
		len(graph.Edges),
		graph.Folded,
		graph.Workloads,
	)
}
