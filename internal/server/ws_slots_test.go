package server

import (
	"context"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type stubborn struct {
	Backend

	entered chan string
}

func (s *stubborn) Subscribe(
	ctx context.Context,
	group, version, resource, namespace string,
	limit int,
	filters []api.RowFilter,
) (*resources.Subscription, error) {
	s.entered <- namespace
	if namespace != "" {
		return s.Backend.Subscribe(ctx, group, version, resource, namespace, limit, filters)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func deploymentsIn(subID, namespace string) api.ClientMsg {
	return api.ClientMsg{
		Type:      "subscribe",
		SubID:     subID,
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: namespace,
	}
}

func TestATableReplacedWhileItWasStillBuildingGivesItsSlotToTheNextOne(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("payments", "ledger"))
	sess, client, ctx := rawSession(t, mgr)
	sess.snapshots = newWorkBudget(defaultSnapshotLimit, defaultIdentitySnapshotLimit)
	backend := &stubborn{Backend: mgr, entered: make(chan string, 3)}
	first := sess.claim(tables, "main#0")
	go sess.buildSub(ctx, backend, deploymentsIn("main#0", ""), first)
	<-backend.entered
	second := sess.claim(tables, "main#0")
	go sess.buildSub(ctx, backend, deploymentsIn("main#0", ""), second)
	<-backend.entered

	third := sess.claim(tables, "main#0")
	go sess.buildSub(ctx, backend, deploymentsIn("main#0", "payments"), third)

	got := readMsg(ctx, t, client)
	if got.Type != "snapshot" || got.SubID != "main#0" {
		t.Fatalf("frame = %+v, want the payments snapshot once the replaced builds let go", got)
	}
	if len(got.Rows) != 1 || got.Rows[0].Name != "ledger" {
		t.Fatalf("rows = %+v, want the ledger deployment", got.Rows)
	}
	sess.drop(tables, "main#0")
	assertReleasedBudget(t, sess.snapshots)
}

func TestAnUnsubscribedTableStopsBuildingAndFreesItsSlot(t *testing.T) {
	mgr, _ := testManager(t)
	sess, _, ctx := rawSession(t, mgr)
	sess.snapshots = newWorkBudget(1, 1)
	backend := &stubborn{Backend: mgr, entered: make(chan string, 1)}
	built := make(chan struct{})
	gen := sess.claim(tables, "main#0")
	go func() {
		sess.buildSub(ctx, backend, deploymentsIn("main#0", ""), gen)
		close(built)
	}()
	<-backend.entered

	sess.drop(tables, "main#0")

	select {
	case <-built:
	case <-time.After(5 * time.Second):
		t.Fatal("the dropped table kept building")
	}
	assertReleasedBudget(t, sess.snapshots)
}

func TestATableWaitsForASlotRatherThanBeingRefused(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("payments", "ledger"))
	sess, client, ctx := rawSession(t, mgr)
	sess.snapshots = newWorkBudget(1, 1)
	held, ok := sess.snapshots.claim("someone-else", 1)
	if !ok {
		t.Fatal("the first reservation was refused")
	}
	backend := &stubborn{Backend: mgr, entered: make(chan string, 1)}
	gen := sess.claim(tables, "main#0")
	go sess.buildSub(ctx, backend, deploymentsIn("main#0", "payments"), gen)

	held()

	if got := readMsg(ctx, t, client); got.Type != "snapshot" {
		t.Fatalf("frame = %+v, want a snapshot once the slot came back", got)
	}
	sess.drop(tables, "main#0")
	assertReleasedBudget(t, sess.snapshots)
}

func TestATableThatNeverGetsASlotIsToldSo(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	sess.snapshots = newWorkBudget(1, 1)
	sess.slotWait = 20 * time.Millisecond
	held, ok := sess.snapshots.claim("someone-else", 1)
	if !ok {
		t.Fatal("the first reservation was refused")
	}
	t.Cleanup(held)

	sess.buildSub(ctx, mgr, deploymentsIn("main#0", "default"), sess.claim(tables, "main#0"))

	got := readMsg(ctx, t, client)
	if got.Type != msgError || got.Message != "table snapshot capacity is full; try again later" {
		t.Fatalf("frame = %+v, want the capacity refusal", got)
	}
	if len(sess.tables) != 0 {
		t.Fatal("the refused table kept its subscription slot")
	}
}

func TestTheSlotWaitIsLongEnoughForASlowBuildAndShortEnoughToAnswer(t *testing.T) {
	if snapshotWait < 2*time.Second || snapshotWait > 30*time.Second {
		t.Fatalf("snapshotWait = %s, want between 2s and 30s", snapshotWait)
	}
	if (&wsSession{}).waitForSlot() != snapshotWait {
		t.Fatal("a session without its own wait did not use the default")
	}
}

func TestAWaitForABudgetSlotEndsWithItsContext(t *testing.T) {
	budget := newWorkBudget(1, 1)
	held, ok := budget.claim("alice", 1)
	if !ok {
		t.Fatal("the first reservation was refused")
	}
	t.Cleanup(held)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, ok := budget.await(ctx, "bob"); ok {
		t.Fatal("a canceled wait was handed a slot that was never freed")
	}
}

func TestAWaitForABudgetSlotGetsTheFreedOne(t *testing.T) {
	budget := newWorkBudget(1, 1)
	held, ok := budget.claim("alice", 1)
	if !ok {
		t.Fatal("the first reservation was refused")
	}
	got := make(chan bool, 1)
	go func() {
		release, ok := budget.await(t.Context(), "bob")
		if ok {
			release()
		}
		got <- ok
	}()

	held()

	select {
	case ok := <-got:
		if !ok {
			t.Fatal("the waiter was refused after the slot came back")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter never woke when the slot came back")
	}
	assertReleasedBudget(t, budget)
}

func TestANilBudgetNeverMakesAnyoneWait(t *testing.T) {
	var budget *workBudget

	release, ok := budget.await(t.Context(), "alice")

	if !ok {
		t.Fatal("a missing budget refused work")
	}
	release()
}
