package server

import (
	"context"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type panickingManagerFleet struct {
	*fleet
}

func (p *panickingManagerFleet) Manager(string) Backend {
	panic("manager registry changed")
}

func TestCancelingAFleetReadReportsTheRequestCancellation(t *testing.T) {
	held := &fleet{
		held:     []api.OpenCluster{{ID: mk1, Context: "p-mk1", Active: true}},
		active:   mk1,
		backends: map[string]Backend{mk1: &surveying{}},
	}
	srv := New(held, testAssets(), testToken)
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})
	done := make(chan []clusterAnswer[string], 1)
	go func() {
		done <- eachOpenClusterWithin(ctx, srv, time.Minute, func(
			context.Context,
			api.OpenCluster,
			Backend,
		) string {
			close(started)
			defer close(stopped)
			<-release
			return "late"
		})
	}()

	<-started
	cancel()
	found := <-done
	close(release)
	<-stopped

	if len(found) != 1 {
		t.Fatalf("answers = %d, want one", len(found))
	}
	if found[0].failure != context.Canceled.Error() {
		t.Fatalf("failure = %q, want %q", found[0].failure, context.Canceled)
	}
}

func TestAFleetManagerLookupPanicDoesNotLoseTheOtherClusters(t *testing.T) {
	held := &panickingManagerFleet{fleet: &fleet{
		held: []api.OpenCluster{
			{ID: mk1, Context: "p-mk1", Active: true},
			{ID: mk2, Context: "p-mk2"},
		},
		active: mk1,
	}}
	srv := New(held, testAssets(), testToken)

	found := eachOpenClusterWithin(t.Context(), srv, time.Second, func(
		context.Context,
		api.OpenCluster,
		Backend,
	) string {
		return "unreachable"
	})

	if len(found) != 2 {
		t.Fatalf("answers = %d, want both clusters", len(found))
	}
	for _, answer := range found {
		if answer.failure != fleetReadFailure {
			t.Fatalf("%s failure = %q, want the bounded failure", answer.context, answer.failure)
		}
		if answer.answer != "" {
			t.Fatalf("%s answer = %q after the manager lookup panicked", answer.context, answer.answer)
		}
	}
}
