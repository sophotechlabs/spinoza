package server

import (
	"context"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func rowEvent(kind, uid string) resources.Event {
	return resources.Event{Kind: kind, Row: api.Row{UID: uid, Name: uid, Namespace: "prod"}, UID: uid}
}

func liveEntry(sess *wsSession, subID string, gen uint64) {
	sess.mu.Lock()
	sess.tables[subID] = &entry{gen: gen}
	sess.mu.Unlock()
}

func TestABatchCarriesEveryChangeThatWasWaiting(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	liveEntry(sess, "main", 1)
	more := make(chan resources.Event, 2)
	more <- rowEvent("modified", "u-2")
	more <- rowEvent("deleted", "u-3")

	if !sess.writeBatch("main", 1, rowEvent("added", "u-1"), more) {
		t.Fatal("writeBatch reported the subscription as stale")
	}

	msg := readMsg(ctx, t, client)
	if msg.Type != "batch" {
		t.Fatalf("type = %q, want batch", msg.Type)
	}
	if len(msg.Changes) != 3 {
		t.Fatalf("changes = %d, want the three that were waiting", len(msg.Changes))
	}
	if msg.Changes[2].UID != "u-3" {
		t.Fatalf("last change = %+v, want the deletion of u-3", msg.Changes[2])
	}
}

func TestAnErrorArrivesOnItsOwnRatherThanInABatch(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	liveEntry(sess, "main", 1)
	failure := resources.Event{Kind: msgError, Message: "the watch broke"}

	if !sess.writeBatch("main", 1, failure, make(chan resources.Event)) {
		t.Fatal("writeBatch reported the subscription as stale")
	}

	msg := readMsg(ctx, t, client)
	if msg.Type != msgError {
		t.Fatalf("type = %q, want an error frame", msg.Type)
	}
	if msg.Message != "the watch broke" {
		t.Fatalf("message = %q", msg.Message)
	}
}

func TestAnErrorMidBatchFlushesWhatWasAlreadyGathered(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	liveEntry(sess, "main", 1)
	more := make(chan resources.Event, 2)
	more <- rowEvent("modified", "u-2")
	more <- resources.Event{Kind: msgError, Message: "the watch broke"}

	if !sess.writeBatch("main", 1, rowEvent("added", "u-1"), more) {
		t.Fatal("writeBatch reported the subscription as stale")
	}

	batch := readMsg(ctx, t, client)
	if batch.Type != "batch" || len(batch.Changes) != 2 {
		t.Fatalf("first frame = %+v, want the two changes gathered before the error", batch)
	}
	failure := readMsg(ctx, t, client)
	if failure.Type != msgError {
		t.Fatalf("second frame = %+v, want the error", failure)
	}
}

func TestAClosedStreamFlushesTheLastBatch(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	liveEntry(sess, "main", 1)
	more := make(chan resources.Event, 1)
	more <- rowEvent("modified", "u-2")
	close(more)

	if !sess.writeBatch("main", 1, rowEvent("added", "u-1"), more) {
		t.Fatal("writeBatch reported the subscription as stale")
	}

	msg := readMsg(ctx, t, client)
	if len(msg.Changes) != 2 {
		t.Fatalf("changes = %d, want both before the stream closed", len(msg.Changes))
	}
}

func TestABatchStopsAtItsCap(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)
	liveEntry(sess, "main", 1)
	more := make(chan resources.Event, maxBatch+10)
	for i := range maxBatch + 9 {
		more <- rowEvent("added", string(rune('a'+i%26))+string(rune('a'+i/26)))
	}

	if !sess.writeBatch("main", 1, rowEvent("added", "first"), more) {
		t.Fatal("writeBatch reported the subscription as stale")
	}

	msg := readMsg(ctx, t, client)
	if len(msg.Changes) != maxBatch {
		t.Fatalf("changes = %d, want the cap of %d", len(msg.Changes), maxBatch)
	}
}

func TestABatchForAReplacedSubscriptionIsDropped(t *testing.T) {
	mgr, _ := testManager(t)
	sess, _, _ := rawSession(t, mgr)
	liveEntry(sess, "main", 2)

	if sess.writeBatch("main", 1, rowEvent("added", "u-1"), make(chan resources.Event)) {
		t.Fatal("writeBatch wrote under a generation that had been replaced")
	}
}

func TestAnErrorAfterAReplacedBatchIsDropped(t *testing.T) {
	mgr, _ := testManager(t)
	sess, _, _ := rawSession(t, mgr)
	liveEntry(sess, "main", 2)
	more := make(chan resources.Event, 1)
	more <- resources.Event{Kind: msgError, Message: "the old watch broke"}

	if sess.writeBatch("main", 1, rowEvent("added", "u-1"), more) {
		t.Fatal("the stale batch was written before its error")
	}
}

func TestTheRelayStopsWhenTheResyncChannelCloses(t *testing.T) {
	mgr, _ := testManager(t)
	sess, _, _ := rawSession(t, mgr)
	liveEntry(sess, "main", 1)
	resync := make(chan struct{})
	close(resync)
	sub := &resources.Subscription{Events: make(chan resources.Event), Resync: resync}

	done := make(chan struct{})
	go func() {
		sess.relay("main", 1, sub)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the relay did not stop when its resync channel closed")
	}
}

func TestTheRelayStopsWhenTheEventChannelCloses(t *testing.T) {
	mgr, _ := testManager(t)
	sess, _, _ := rawSession(t, mgr)
	liveEntry(sess, "main", 1)
	events := make(chan resources.Event)
	close(events)
	sub := &resources.Subscription{Events: events, Resync: make(chan struct{})}

	done := make(chan struct{})
	go func() {
		sess.relay("main", 1, sub)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the relay did not stop when its event channel closed")
	}
}

func TestTheRelayStopsWhenTheSessionIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	sess := &wsSession{ctx: ctx}
	sub := &resources.Subscription{Events: make(chan resources.Event), Resync: make(chan struct{})}
	done := make(chan struct{})

	go func() {
		sess.relay("main", 1, sub)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the canceled session left its resource relay running")
	}
}

func TestTheRelayStopsOnceTheSubscriptionIsReplaced(t *testing.T) {
	mgr, _ := testManager(t)
	sess, _, _ := rawSession(t, mgr)
	liveEntry(sess, "main", 2)
	events := make(chan resources.Event, 1)
	events <- rowEvent("added", "u-1")
	sub := &resources.Subscription{Events: events, Resync: make(chan struct{})}

	done := make(chan struct{})
	go func() {
		sess.relay("main", 1, sub)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the relay kept going for a subscription that had been replaced")
	}
}

func TestTheRelayStopsWhenAResyncBelongsToAReplacedSubscription(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, _, _ := rawSession(t, mgr)
	sub, err := mgr.Subscribe(t.Context(), "apps", "v1", "deployments", "default", 1, nil)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	liveEntry(sess, "main", 2)
	sub.SetLimit(2)
	done := make(chan struct{})
	go func() {
		sess.relay("main", 1, sub)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stale relay kept running after its resync was rejected")
	}
}

func TestAMessageTypeNobodyKnowsIsIgnored(t *testing.T) {
	mgr, _ := testManager(t)
	sess, _, _ := rawSession(t, mgr)

	sess.handle(api.ClientMsg{Type: "sing", SubID: "main"})

	sess.mu.Lock()
	held := len(sess.tables)
	sess.mu.Unlock()
	if held != 0 {
		t.Fatalf("tables = %d, want an unknown message to change nothing", held)
	}
}
