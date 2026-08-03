package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	ts := httptest.NewServer(New(fixed(mgr), testAssets()).Handler())
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
		conn: server,
		ctx:  ctx,
		mgr:  mgr,
		subs: map[string]*subEntry{},
		logs: map[string]*logs.Stream{},
	}
	return sess, client, ctx
}

func TestResyncSendsAFreshSnapshot(t *testing.T) {
	mgr, dyn := testManager(t, newDeployment("default", "web"))
	sess, client, ctx := rawSession(t, mgr)

	sub, err := mgr.Subscribe("apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	sess.subs["main"] = &subEntry{sub: sub, gen: 1}

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

	sub, err := mgr.Subscribe("apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	sess.subs["main"] = &subEntry{sub: sub, gen: 1}

	_, createErr := dyn.Resource(depGVR).Namespace("default").
		Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if createErr != nil {
		t.Fatalf("create: %v", createErr)
	}
	waitForRows(t, sub, 2)

	sess.sendResync("main", 1, sub)

	select {
	case ev := <-sub.Events:
		t.Fatalf("a stale event survived the resync and would be applied after it: %+v", ev)
	default:
	}
}

func TestResyncIsSkippedForAReplacedSubscription(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	sess, client, ctx := rawSession(t, mgr)

	sub, err := mgr.Subscribe("apps", "v1", "deployments", "default")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)
	sess.subs["main"] = &subEntry{sub: sub, gen: 2}

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
		if len(sub.Snapshot()) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("cache never reached %d rows", want)
}
