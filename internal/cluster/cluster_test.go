package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/resources"
)

func stubList() ([]string, string, error) {
	return []string{"alpha", "beta"}, "beta", nil
}

type recorder struct {
	names   []string
	live    []context.Context
	failOn  string
	failErr error
}

func (r *recorder) build(ctx context.Context, name string) (*resources.Manager, string, error) {
	if r.failErr != nil && name == r.failOn {
		return nil, "", r.failErr
	}
	r.names = append(r.names, name)
	r.live = append(r.live, ctx)
	resolved := name
	if resolved == "" {
		resolved = "default-context"
	}
	return resources.NewManager(ctx, resources.Deps{}), resolved, nil
}

func newTestCluster(t *testing.T, rec *recorder) *Cluster {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster, err := newCluster(ctx, rec.build, stubList)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return cluster
}

func TestNewBuildsTheDefaultContext(t *testing.T) {
	rec := &recorder{}

	cluster := newTestCluster(t, rec)

	if len(rec.names) != 1 || rec.names[0] != "" {
		t.Fatalf("built %v, want one build of the kubeconfig default", rec.names)
	}
	if cluster.Current() != "default-context" {
		t.Fatalf("current = %q", cluster.Current())
	}
	if cluster.Manager() == nil {
		t.Fatal("no manager after construction")
	}
}

func TestUseSwapsTheManager(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)
	first := cluster.Manager()

	err := cluster.Use("p-mk1")
	if err != nil {
		t.Fatalf("use: %v", err)
	}

	if cluster.Manager() == first {
		t.Fatal("the manager was not replaced")
	}
	if cluster.Current() != "p-mk1" {
		t.Fatalf("current = %q", cluster.Current())
	}
}

func TestUseCancelsThePreviousManager(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)

	err := cluster.Use("p-mk1")
	if err != nil {
		t.Fatalf("use: %v", err)
	}

	select {
	case <-rec.live[0].Done():
	default:
		t.Fatal("the old context stayed live; its informers and forwards would keep running")
	}
	select {
	case <-rec.live[1].Done():
		t.Fatal("the new context was canceled")
	default:
	}
}

func TestAFailedUseKeepsTheWorkingManager(t *testing.T) {
	rec := &recorder{failOn: "gone", failErr: errors.New("context \"gone\" does not exist")}
	cluster := newTestCluster(t, rec)
	before := cluster.Manager()

	err := cluster.Use("gone")

	if err == nil {
		t.Fatal("expected the failure to surface")
	}
	if cluster.Manager() != before {
		t.Fatal("a failed switch replaced the working manager")
	}
	if cluster.Current() != "default-context" {
		t.Fatalf("current = %q, want the old context kept", cluster.Current())
	}
	select {
	case <-rec.live[0].Done():
		t.Fatal("a failed switch canceled the working manager")
	default:
	}
}

func TestNewSurfacesABuildFailure(t *testing.T) {
	rec := &recorder{failOn: "", failErr: errors.New("kubeconfig is unreadable")}

	_, err := newCluster(context.Background(), rec.build, stubList)

	if err == nil {
		t.Fatal("expected the build failure to surface")
	}
}

func TestContextsReportsTheCurrentSelection(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)

	list := cluster.Contexts()

	if list.Current != "default-context" {
		t.Fatalf("current = %q, want the context spinoza actually connected to", list.Current)
	}
}

func TestContextsSurfacesAKubeconfigFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster, err := newCluster(ctx, (&recorder{}).build, func() ([]string, string, error) {
		return nil, "", errors.New("kubeconfig is unreadable")
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	list := cluster.Contexts()

	if list.Error == "" {
		t.Fatal("an unreadable kubeconfig was reported as an empty context list")
	}
	if list.Current != "default-context" {
		t.Fatalf("current = %q, want the connected context even when listing fails", list.Current)
	}
}

type gatedBuilder struct {
	gates   map[string]chan struct{}
	entered chan string
}

func newGatedBuilder(slow string) *gatedBuilder {
	return &gatedBuilder{
		gates:   map[string]chan struct{}{slow: make(chan struct{})},
		entered: make(chan string, 4),
	}
}

func (g *gatedBuilder) build(ctx context.Context, name string) (*resources.Manager, string, error) {
	g.entered <- name
	gate, ok := g.gates[name]
	if ok {
		<-gate
	}
	return resources.NewManager(ctx, resources.Deps{}), name, nil
}

func (g *gatedBuilder) waitFor(t *testing.T, name string) {
	t.Helper()
	for {
		select {
		case entered := <-g.entered:
			if entered == name {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("the builder was never asked for %q", name)
		}
	}
}

func TestTheLastRequestedContextWinsEvenIfItBuildsFirst(t *testing.T) {
	gated := newGatedBuilder("slow")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster, err := newCluster(ctx, gated.build, stubList)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cluster.Use("slow")
	}()
	gated.waitFor(t, "slow")

	fastErr := cluster.Use("fast")
	if fastErr != nil {
		t.Fatalf("use fast: %v", fastErr)
	}

	close(gated.gates["slow"])
	slowErr := <-done
	if slowErr != nil {
		t.Fatalf("use slow: %v", slowErr)
	}

	if cluster.Current() != "fast" {
		t.Fatalf("current = %q, want the context requested last", cluster.Current())
	}
}

func TestASupersededSwitchDoesNotStrandItsManager(t *testing.T) {
	gated := newGatedBuilder("slow")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster, err := newCluster(ctx, gated.build, stubList)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cluster.Use("slow")
	}()
	gated.waitFor(t, "slow")

	useErr := cluster.Use("fast")
	if useErr != nil {
		t.Fatalf("use fast: %v", useErr)
	}
	winner := cluster.Manager()

	close(gated.gates["slow"])
	if slowErr := <-done; slowErr != nil {
		t.Fatalf("use slow: %v", slowErr)
	}

	if cluster.Manager() != winner {
		t.Fatal("a superseded switch replaced the installed manager")
	}
}

func TestAFailedSwitchKeepsTheWorkingCluster(t *testing.T) {
	rec := &recorder{failOn: "broken", failErr: errors.New("context \"broken\" lists no resource types")}
	cluster := newTestCluster(t, rec)
	working := cluster.Manager()

	err := cluster.Use("broken")

	if err == nil {
		t.Fatal("switching to an unusable context reported success")
	}
	if cluster.Manager() != working {
		t.Fatal("the working cluster's manager was replaced by the unusable one")
	}
	if cluster.Current() != "default-context" {
		t.Fatalf("current = %q, want the working context to still be in force", cluster.Current())
	}
}

func TestAFailedSwitchLeavesTheWorkingInformersRunning(t *testing.T) {
	rec := &recorder{failOn: "broken", failErr: errors.New("unreachable")}
	cluster := newTestCluster(t, rec)
	live := rec.live[0]

	useErr := cluster.Use("broken")
	if useErr == nil {
		t.Fatal("expected the switch to fail")
	}

	select {
	case <-live.Done():
		t.Fatal("the working cluster's context was canceled by a switch that never took effect")
	default:
	}
}
