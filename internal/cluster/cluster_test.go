package cluster

import (
	"context"
	"errors"
	"testing"

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
	return resources.NewManager(ctx, nil, nil, nil, nil, nil, nil, nil, nil, nil), resolved, nil
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
