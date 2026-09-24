package server

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func assertReleasedBudget(t *testing.T, budget *workBudget) {
	t.Helper()
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.used != 0 || len(budget.byIdentity) != 0 {
		t.Fatalf("budget used/identities = %d/%v, want every reservation released", budget.used, budget.byIdentity)
	}
}

func TestALogOpenFailureReleasesCapacityAndTheSameIDCanRecover(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	sess.logStreams = newWorkBudget(1, 1)
	msg := api.ClientMsg{SubID: "logs", Namespace: "default", Name: "web", Container: "app", TailLines: 100}
	refused := &awkward{Backend: mgr, logsErr: errors.New("pod log transport refused")}
	sess.buildLogs(refused, msg, sess.claim(streams, msg.SubID))
	if got := readMsg(ctx, t, client); got.Type != msgError || got.SubID != msg.SubID || got.Message != refused.logsErr.Error() {
		t.Fatalf("frame = %+v, want the exact opening failure", got)
	}
	assertReleasedBudget(t, sess.logStreams)
	if sess.resourceOf(streams, msg.SubID) != nil || len(sess.logs) != 0 {
		t.Fatal("the failed log request retained its subscription slot")
	}
	sess.buildLogs(mgr, msg, sess.claim(streams, msg.SubID))
	if got := readMsg(ctx, t, client); got.Type != "log-open" || got.SubID != msg.SubID {
		t.Fatalf("frame = %+v, want the same ID to open after recovery", got)
	}
	sess.drop(streams, msg.SubID)
	sess.drop(streams, msg.SubID)
	assertReleasedBudget(t, sess.logStreams)
}

func TestAWorkloadSelectorFailureReleasesItsWholeLogReservation(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	sess.logStreams = newWorkBudget(maxWorkloadLogStreams, maxWorkloadLogStreams)
	msg := api.ClientMsg{SubID: "workload", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default", Name: "web", TailLines: 100}
	refused := &awkward{Backend: mgr, selectorErr: errors.New("deployment selector is unavailable")}
	sess.buildLogs(refused, msg, sess.claim(streams, msg.SubID))
	if got := readMsg(ctx, t, client); got.Type != msgError || got.Message != refused.selectorErr.Error() {
		t.Fatalf("frame = %+v, want the selector failure", got)
	}
	assertReleasedBudget(t, sess.logStreams)
	release, accepted := sess.logStreams.claim("another-viewer", maxWorkloadLogStreams)
	if !accepted {
		t.Fatal("a refused selector exhausted capacity for the next viewer")
	}
	release()
}

func TestASaturatedLogBudgetRefusesWithoutLeakingAConnectionSlot(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	sess.logStreams = newWorkBudget(1, 1)
	release, accepted := sess.logStreams.claim("other", 1)
	if !accepted {
		t.Fatal("the initial reservation was refused")
	}
	t.Cleanup(release)
	msg := api.ClientMsg{SubID: "logs", Namespace: "default", Name: "web", Container: "app", TailLines: 100}
	sess.buildLogs(mgr, msg, sess.claim(streams, msg.SubID))
	if got := readMsg(ctx, t, client); got.Type != msgError || got.Message != "log stream capacity is full; close another log stream and try again" {
		t.Fatalf("frame = %+v, want capacity refusal", got)
	}
	if len(sess.logs) != 0 {
		t.Fatal("the capacity refusal retained a connection slot")
	}
	release()
	sess.buildLogs(mgr, msg, sess.claim(streams, msg.SubID))
	if got := readMsg(ctx, t, client); got.Type != "log-open" {
		t.Fatalf("frame = %+v, want an open stream once capacity returns", got)
	}
	sess.drop(streams, msg.SubID)
	assertReleasedBudget(t, sess.logStreams)
}

func TestAResyncCapacityRefusalKeepsTheSubscriptionAndRecoversItsSnapshot(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, client, ctx := rawSession(t, mgr)
	sess.snapshots = newWorkBudget(1, 1)
	sub, err := mgr.Subscribe(ctx, "apps", "v1", "deployments", "default", 0, nil)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	gen := sess.claim(tables, "table")
	if !sess.adopt(tables, "table", gen, sub) {
		t.Fatal("the initial subscription was refused")
	}
	release, accepted := sess.snapshots.claim("other", 1)
	if !accepted {
		t.Fatal("the initial reservation was refused")
	}
	t.Cleanup(release)
	if !sess.sendResync("table", gen, sub) {
		t.Fatal("a transient capacity refusal ended the subscription")
	}
	if got := readMsg(ctx, t, client); got.Type != msgError || got.Message != "table snapshot capacity is full; try again later" {
		t.Fatalf("frame = %+v, want the bounded resync failure", got)
	}
	if sess.resourceOf(tables, "table") != sub {
		t.Fatal("the capacity refusal discarded the live subscription")
	}
	release()
	if !sess.sendResync("table", gen, sub) {
		t.Fatal("the subscription could not resync after capacity returned")
	}
	got := readMsg(ctx, t, client)
	if got.Type != "snapshot" || got.SubID != "table" || len(got.Rows) != 1 || got.Rows[0].Name != "web" || got.Total != 1 {
		t.Fatalf("frame = %+v, want the original subscription's current complete snapshot", got)
	}
	assertReleasedBudget(t, sess.snapshots)
}

func TestALogOpeningAfterUnsubscribeClosesAndReleasesItsReservation(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	sess.logStreams = newWorkBudget(1, 1)
	blocked := &awkward{Backend: mgr, hold: make(chan struct{}), entered: make(chan struct{}, 1)}
	msg := api.ClientMsg{SubID: "logs", Namespace: "default", Name: "web", Container: "app", TailLines: 100}
	gen := sess.claim(streams, msg.SubID)
	release := sync.OnceFunc(func() {
		close(blocked.hold)
	})
	t.Cleanup(release)
	done := make(chan struct{})
	go func() {
		sess.buildLogs(blocked, msg, gen)
		close(done)
	}()
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("the log request never reached the backend")
	}
	sess.drop(streams, msg.SubID)
	release()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("an unsubscribed log request retained its opening goroutine")
	}
	assertReleasedBudget(t, sess.logStreams)
	sess.write(ctx, api.ServerMsg{Type: "marker"})
	if got := readMsg(ctx, t, client); got.Type != "marker" {
		t.Fatalf("frame = %+v, want no late open, output or end after unsubscribe", got)
	}
}
