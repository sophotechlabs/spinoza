package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func openFeed(t *testing.T, mgr *resources.Manager) (*websocket.Conn, context.Context) {
	t.Helper()
	ts := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.SetReadLimit(1 << 24)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn, ctx
}

func sendClient(ctx context.Context, t *testing.T, conn *websocket.Conn, msg api.ClientMsg) {
	t.Helper()
	err := wsjson.Write(ctx, conn, msg)
	if err != nil {
		t.Fatalf("write %s: %v", msg.Type, err)
	}
}

func subscribeMain(ctx context.Context, t *testing.T, conn *websocket.Conn, subID string) {
	t.Helper()
	sendClient(ctx, t, conn, api.ClientMsg{
		Type:      "subscribe",
		SubID:     subID,
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "default",
	})
}

func TestResubscribingDropsTheOldSubscriptionsEvents(t *testing.T) {
	mgr, dyn := testManager(t, newDeployment("default", "web"))
	conn, ctx := openFeed(t, mgr)

	subscribeMain(ctx, t, conn, "main")
	if readMsg(ctx, t, conn).Type != "snapshot" {
		t.Fatal("expected the first snapshot")
	}
	subscribeMain(ctx, t, conn, "main")
	if readMsg(ctx, t, conn).Type != "snapshot" {
		t.Fatal("expected the second snapshot")
	}

	_, err := dyn.Resource(depGVR).Namespace("default").
		Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	first := readMsg(ctx, t, conn)
	if first.Type != "added" || first.Row == nil || first.Row.Name != "api" {
		t.Fatalf("first delta = %+v", first)
	}

	sendClient(ctx, t, conn, api.ClientMsg{Type: "unsubscribe", SubID: "main"})
	subscribeMain(ctx, t, conn, "fence")
	fence := readMsg(ctx, t, conn)
	if fence.Type != "snapshot" || fence.SubID != "fence" {
		t.Fatalf("message = %s/%s, want only one delta from the live subscription", fence.Type, fence.SubID)
	}
}

func TestEventsStopAfterUnsubscribe(t *testing.T) {
	mgr, dyn := testManager(t, newDeployment("default", "web"))
	conn, ctx := openFeed(t, mgr)

	subscribeMain(ctx, t, conn, "main")
	if readMsg(ctx, t, conn).Type != "snapshot" {
		t.Fatal("expected a snapshot")
	}
	sendClient(ctx, t, conn, api.ClientMsg{Type: "unsubscribe", SubID: "main"})
	subscribeMain(ctx, t, conn, "fence")
	if readMsg(ctx, t, conn).SubID != "fence" {
		t.Fatal("expected the fence snapshot, which proves the unsubscribe was handled")
	}

	const churn = 5
	for i := range churn {
		_, err := dyn.Resource(depGVR).Namespace("default").
			Create(ctx, newDeployment("default", fmt.Sprintf("api-%d", i)), metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	for range churn {
		msg := readMsg(ctx, t, conn)
		if msg.SubID != "fence" {
			t.Fatalf("message for %q arrived after it was unsubscribed: %+v", msg.SubID, msg)
		}
	}
}

func rawSession(t *testing.T, mgr *resources.Manager) (*wsSession, *websocket.Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	accepted := make(chan *websocket.Conn, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		accepted <- conn
		<-ctx.Done()
	}))
	t.Cleanup(ts.Close)

	client, _, dialErr := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if dialErr != nil {
		t.Fatalf("dial: %v", dialErr)
	}
	t.Cleanup(func() { _ = client.CloseNow() })

	server := <-accepted
	sess := &wsSession{
		conn:   server,
		ctx:    ctx,
		mgr:    mgr,
		tables: map[string]*entry{},
		logs:   map[string]*entry{},
	}
	return sess, client, ctx
}

func TestResyncSendsAFreshSnapshot(t *testing.T) {
	mgr, dyn := testManager(t, newDeployment("default", "web"))
	sess, client, ctx := rawSession(t, mgr)

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	sess.tables["main"] = &entry{resource: sub, gen: 1}

	_, createErr := dyn.Resource(depGVR).Namespace("default").
		Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if createErr != nil {
		t.Fatalf("create: %v", createErr)
	}
	waitForRows(t, sub, 2)

	if !sess.sendResync("main", 1, sub) {
		t.Fatal("sendResync reported the subscription as stale")
	}

	msg := readMsg(ctx, t, client)
	if msg.Type != "snapshot" {
		t.Fatalf("type = %q, want a snapshot", msg.Type)
	}
	if msg.SubID != "main" {
		t.Fatalf("subId = %q", msg.SubID)
	}
	if len(msg.Rows) != 2 {
		t.Fatalf("rows = %d, want the current cache contents", len(msg.Rows))
	}
}

func TestResyncDrainsTheStaleEventsFirst(t *testing.T) {
	mgr, dyn := testManager(t, newDeployment("default", "web"))
	sess, _, ctx := rawSession(t, mgr)

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	sess.tables["main"] = &entry{resource: sub, gen: 1}

	_, createErr := dyn.Resource(depGVR).Namespace("default").
		Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if createErr != nil {
		t.Fatalf("create: %v", createErr)
	}
	waitForRows(t, sub, 2)
	waitForQueuedEvent(t, sub)

	sess.sendResync("main", 1, sub)

	select {
	case ev := <-sub.Events:
		t.Fatalf("a stale event survived the resync and would be applied after it: %+v", ev)
	default:
	}
}

func waitForQueuedEvent(t *testing.T, sub *resources.Subscription) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(sub.Events) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no event was ever queued, so the drain would have had nothing to prove")
}

func TestResyncIsSkippedForAReplacedSubscription(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, client, ctx := rawSession(t, mgr)

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	sess.tables["main"] = &entry{resource: sub, gen: 2}

	if sess.sendResync("main", 1, sub) {
		t.Fatal("an old generation was allowed to write")
	}

	sess.write(ctx, api.ServerMsg{Type: "error", SubID: "fence", Message: "fence"})
	msg := readMsg(ctx, t, client)
	if msg.SubID != "fence" {
		t.Fatalf("a stale resync reached the browser: %+v", msg)
	}
}

func waitForRows(t *testing.T, sub *resources.Subscription, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := sub.Snapshot()
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		if len(rows) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("cache never reached %d rows", want)
}

func TestAdoptRefusesASupersededSubscription(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, _, _ := rawSession(t, mgr)

	first := sess.claim(tables, "main")
	second := sess.claim(tables, "main")

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)

	if sess.adopt(tables, "main", first, sub) {
		t.Fatal("a subscription built for an older request was installed")
	}
	if !sess.adopt(tables, "main", second, sub) {
		t.Fatal("the newest request could not install its subscription")
	}
}

func TestAdoptRefusesASubscriptionCancelledWhileBuilding(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, _, _ := rawSession(t, mgr)

	gen := sess.claim(tables, "main")
	sess.drop(tables, "main")

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)

	if sess.adopt(tables, "main", gen, sub) {
		t.Fatal("a subscription the client already dropped was installed")
	}
}

func TestClaimClosesThePreviousSubscription(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, _, _ := rawSession(t, mgr)

	first := sess.claim(tables, "main")
	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if !sess.adopt(tables, "main", first, sub) {
		t.Fatal("adopt refused the current generation")
	}

	sess.claim(tables, "main")

	select {
	case _, open := <-sub.Events:
		if open {
			t.Fatal("the replaced subscription is still delivering events")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the replaced subscription was never closed")
	}
}

func TestUnsubscribeIsSafeBeforeTheSubscriptionLands(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, _, _ := rawSession(t, mgr)

	sess.claim(tables, "main")
	sess.drop(tables, "main")
	sess.drop(tables, "main")
}

func TestClaimLogsClosesThePreviousStream(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, _, ctx := rawSession(t, mgr)

	gen := sess.claim(streams, "logs")
	stream, err := mgr.Logs(ctx, logs.Request{Namespace: "default", Name: "web", Container: "app"})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !sess.adopt(streams, "logs", gen, stream) {
		t.Fatal("adopt refused the current generation")
	}

	sess.claim(streams, "logs")

	select {
	case _, open := <-stream.Lines:
		if open {
			drainLines(t, stream.Lines)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the replaced log stream was never closed")
	}
}

func drainLines(t *testing.T, lines <-chan string) {
	t.Helper()
	for {
		select {
		case _, open := <-lines:
			if !open {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatal("the replaced log stream was never closed")
		}
	}
}

func TestAdoptLogsRefusesASupersededStream(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, _, ctx := rawSession(t, mgr)

	first := sess.claim(streams, "logs")
	sess.claim(streams, "logs")

	stream, err := mgr.Logs(ctx, logs.Request{Namespace: "default", Name: "web", Container: "app"})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	t.Cleanup(stream.Close)

	if sess.adopt(streams, "logs", first, stream) {
		t.Fatal("a stream built for an older request was installed")
	}
}

func TestAdoptLogsRefusesAStreamCancelledWhileOpening(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, _, ctx := rawSession(t, mgr)

	gen := sess.claim(streams, "logs")
	sess.drop(streams, "logs")

	stream, err := mgr.Logs(ctx, logs.Request{Namespace: "default", Name: "web", Container: "app"})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	t.Cleanup(stream.Close)

	if sess.adopt(streams, "logs", gen, stream) {
		t.Fatal("a stream the client already dropped was installed")
	}
}

func TestFailCurrentStaysSilentOnceSuperseded(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)

	stale := sess.claim(tables, "main")
	sess.claim(tables, "main")
	sess.failCurrent(tables, "main", stale, errors.New("discovery failed"))
	sess.write(ctx, api.ServerMsg{Type: "marker", SubID: "main"})

	msg := readMsg(ctx, t, client)
	if msg.Type != "marker" {
		t.Fatalf("Type = %q, want the superseded error to have been dropped", msg.Type)
	}
}

func TestAFailureCannotLandAfterTheSnapshotThatReplacedIt(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)

	stale := sess.claim(tables, "main")
	sess.writeMu.Lock()
	failed := make(chan struct{})
	go func() {
		defer close(failed)
		sess.failCurrent(tables, "main", stale, errors.New("discovery failed"))
	}()
	time.Sleep(50 * time.Millisecond)

	sess.claim(tables, "main")
	sess.writeLocked(ctx, api.ServerMsg{Type: "snapshot", SubID: "main"})
	sess.writeMu.Unlock()
	<-failed
	sess.write(ctx, api.ServerMsg{Type: "marker", SubID: "main"})

	if first := readMsg(ctx, t, client); first.Type != "snapshot" {
		t.Fatalf("Type = %q, want the replacement snapshot first", first.Type)
	}
	if second := readMsg(ctx, t, client); second.Type != "marker" {
		t.Fatalf("Type = %q, want the stale error dropped once the replacement had written", second.Type)
	}
}

func TestALogFailureCannotLandAfterTheStreamThatReplacedIt(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)

	stale := sess.claim(streams, "logs")
	sess.writeMu.Lock()
	failed := make(chan struct{})
	go func() {
		defer close(failed)
		sess.failCurrent(streams, "logs", stale, errors.New("pods/log is forbidden"))
	}()
	time.Sleep(50 * time.Millisecond)

	sess.claim(streams, "logs")
	sess.writeLocked(ctx, api.ServerMsg{Type: "log", SubID: "logs"})
	sess.writeMu.Unlock()
	<-failed
	sess.write(ctx, api.ServerMsg{Type: "marker", SubID: "logs"})

	if first := readMsg(ctx, t, client); first.Type != "log" {
		t.Fatalf("Type = %q, want the replacement stream first", first.Type)
	}
	if second := readMsg(ctx, t, client); second.Type != "marker" {
		t.Fatalf("Type = %q, want the stale error dropped once the replacement had written", second.Type)
	}
}

func TestFailCurrentLogsStaysSilentOnceSuperseded(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)

	stale := sess.claim(streams, "logs")
	sess.claim(streams, "logs")
	sess.failCurrent(streams, "logs", stale, errors.New("pods/log is forbidden"))
	sess.write(ctx, api.ServerMsg{Type: "marker", SubID: "logs"})

	msg := readMsg(ctx, t, client)
	if msg.Type != "marker" {
		t.Fatalf("Type = %q, want the superseded error to have been dropped", msg.Type)
	}
}

func TestAReplacedLogStreamStopsWritingUnderTheReusedSubID(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)

	stale := sess.claim(streams, "logs")
	stream, err := mgr.Logs(ctx, logs.Request{Namespace: "default", Name: "web", Container: "app"})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !sess.adopt(streams, "logs", stale, stream) {
		t.Fatal("adopt refused the current generation")
	}

	sess.claim(streams, "logs")

	done := make(chan struct{})
	go func() {
		sess.relayLogs("logs", stale, stream)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stale relay never gave up")
	}

	sess.write(ctx, api.ServerMsg{Type: "marker", SubID: "logs"})
	msg := readMsg(ctx, t, client)
	if msg.Type != "marker" {
		t.Fatalf("type = %q, want the replaced pod's lines and end marker to have been dropped", msg.Type)
	}
}

func TestTheCurrentLogStreamStillReachesTheClient(t *testing.T) {
	mgr, _ := testManager(t)
	sess, client, ctx := rawSession(t, mgr)

	gen := sess.claim(streams, "logs")
	stream, err := mgr.Logs(ctx, logs.Request{Namespace: "default", Name: "web", Container: "app"})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !sess.adopt(streams, "logs", gen, stream) {
		t.Fatal("adopt refused the current generation")
	}

	go sess.relayLogs("logs", gen, stream)

	msg := readMsg(ctx, t, client)
	if msg.Type != "log" {
		t.Fatalf("type = %q, want the live stream to deliver its lines", msg.Type)
	}
	if msg.SubID != "logs" {
		t.Fatalf("subId = %q", msg.SubID)
	}
}

func TestASessionBindsTheClusterItWasTrackedUnder(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	swapped, _ := testManager(t, newDeployment("default", "api"))
	holder := &swappableCluster{manager: mgr}
	srv := New(holder, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	holder.swap(swapped)
	conn, _, dialErr := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if dialErr != nil {
		t.Fatalf("dial: %v", dialErr)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	sub := api.ClientMsg{Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default"}
	if writeErr := wsjson.Write(ctx, conn, sub); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	msg := readMsg(ctx, t, conn)
	if len(msg.Rows) != 1 {
		t.Fatalf("rows = %d, want the one row of the cluster in force", len(msg.Rows))
	}
	if msg.Rows[0].Name != "api" {
		t.Fatalf("row = %q, want the swapped-in cluster's row", msg.Rows[0].Name)
	}
}

type swappableCluster struct {
	mu      sync.Mutex
	manager *resources.Manager
}

func (c *swappableCluster) swap(next *resources.Manager) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manager = next
}

func (c *swappableCluster) Manager() Backend {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.manager
}

func (c *swappableCluster) Contexts() api.ContextList {
	return api.ContextList{}
}

func (c *swappableCluster) Use(api.ContextRef) error {
	return nil
}

func (c *swappableCluster) AddKubeconfig(string) error {
	return nil
}

func (c *swappableCluster) RemoveKubeconfig(string) error {
	return nil
}

func (c *swappableCluster) Protect(bool) error {
	return nil
}

func (c *swappableCluster) Protected() bool {
	return false
}
