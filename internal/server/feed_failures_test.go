package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

// awkward is a backend that behaves like the real one except where a test wants
// it to fail, so the paths that only run when the cluster says no are reachable.
type awkward struct {
	Backend

	subscribeErr error
	logsErr      error
	selectorErr  error
}

func (a *awkward) Subscribe(
	ctx context.Context,
	group, version, resource, namespace string,
	limit int,
	filters []api.RowFilter,
) (*resources.Subscription, error) {
	if a.subscribeErr != nil {
		return nil, a.subscribeErr
	}
	return a.Backend.Subscribe(ctx, group, version, resource, namespace, limit, filters)
}

func (a *awkward) Logs(ctx context.Context, req logs.Request) (*logs.Stream, error) {
	if a.logsErr != nil {
		return nil, a.logsErr
	}
	return a.Backend.Logs(ctx, req)
}

func (a *awkward) PodSelector(ctx context.Context, ref api.ObjectRef) (string, error) {
	if a.selectorErr != nil {
		return "", a.selectorErr
	}
	return a.Backend.PodSelector(ctx, ref)
}

func awkwardServer(t *testing.T, broken *awkward) *httptest.Server {
	t.Helper()
	mgr, _ := testManager(t)
	broken.Backend = mgr
	cluster := fixed(mgr)
	cluster.mgr = mgr
	ts := httptest.NewServer(authed(New(&brokenCluster{stubCluster: cluster, backend: broken}, testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	return ts
}

type brokenCluster struct {
	*stubCluster

	backend Backend
}

func (b *brokenCluster) Manager() Backend {
	return b.backend
}

func openAwkwardFeed(t *testing.T, ts *httptest.Server) (context.Context, *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return ctx, conn
}

func TestASubscriptionThatCannotBeBuiltIsReported(t *testing.T) {
	ts := awkwardServer(t, &awkward{subscribeErr: errors.New("deployments is forbidden")})
	ctx, conn := openAwkwardFeed(t, ts)

	sendMsg(ctx, t, conn, api.ClientMsg{
		Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "deployments",
	})

	msg := readMsg(ctx, t, conn)
	if msg.Type != msgError {
		t.Fatalf("type = %q, want an error frame", msg.Type)
	}
	if msg.Message != "deployments is forbidden" {
		t.Fatalf("message = %q, want what the cluster said", msg.Message)
	}
}

func TestALogStreamThatCannotBeOpenedIsReported(t *testing.T) {
	ts := awkwardServer(t, &awkward{logsErr: errors.New("pods/log is forbidden")})
	ctx, conn := openAwkwardFeed(t, ts)

	sendMsg(ctx, t, conn, api.ClientMsg{
		Type: "logs-subscribe", SubID: "logs", Namespace: "prod", Name: "web", Container: "app",
	})

	msg := readMsg(ctx, t, conn)
	if msg.Type != msgError {
		t.Fatalf("type = %q, want an error frame", msg.Type)
	}
	if msg.Message != "pods/log is forbidden" {
		t.Fatalf("message = %q", msg.Message)
	}
}

func TestAWorkloadWhoseSelectorCannotBeReadIsReported(t *testing.T) {
	ts := awkwardServer(t, &awkward{selectorErr: errors.New("deployments/web is forbidden")})
	ctx, conn := openAwkwardFeed(t, ts)

	sendMsg(ctx, t, conn, api.ClientMsg{
		Type:      "logs-subscribe",
		SubID:     "logs",
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "prod",
		Name:      "web",
	})

	msg := readMsg(ctx, t, conn)
	if msg.Type != msgError {
		t.Fatalf("type = %q, want an error frame", msg.Type)
	}
	if msg.Message != "deployments/web is forbidden" {
		t.Fatalf("message = %q", msg.Message)
	}
}

func TestAnEmptySnapshotStillCarriesColumnsAndRows(t *testing.T) {
	if len(columnsOrEmpty(nil)) != 0 {
		t.Fatal("nil columns did not become an empty list")
	}
	if columnsOrEmpty(nil) == nil {
		t.Fatal("the browser iterates columns without a guard")
	}
	if rowsOrEmpty(nil) == nil {
		t.Fatal("the browser iterates rows without a guard")
	}
	kept := []api.Row{{Name: "web"}}
	if len(rowsOrEmpty(kept)) != 1 {
		t.Fatal("rows that were already there were dropped")
	}
	columns := []api.Column{{Name: "Name"}}
	if len(columnsOrEmpty(columns)) != 1 {
		t.Fatal("columns that were already there were dropped")
	}
}

func TestAFrameForASubscriptionThatIsGoneIsNotWritten(t *testing.T) {
	sess := &wsSession{ctx: t.Context(), tables: map[string]*entry{}, logs: map[string]*entry{}}

	if sess.writeCurrent(tables, "missing", 1, api.LogEnd{Type: "log-end"}) {
		t.Fatal("a frame was written for a subscription nobody holds")
	}
}

func TestAFailureForASubscriptionThatIsGoneIsNotWritten(t *testing.T) {
	sess := &wsSession{ctx: t.Context(), tables: map[string]*entry{}, logs: map[string]*entry{}}

	// Nothing to assert beyond it returning quietly: the session has no
	// connection, so writing would panic.
	sess.failCurrent(tables, "missing", 1, errors.New("too late"))
}

func TestAskingForMoreOfSomethingThatIsNotSubscribed(t *testing.T) {
	sess := &wsSession{ctx: t.Context(), tables: map[string]*entry{}, logs: map[string]*entry{}}

	sess.more(api.ClientMsg{Type: "more", SubID: "missing", Limit: 50})
}

func TestAnUnknownClientFrameIsIgnored(t *testing.T) {
	sess := &wsSession{ctx: t.Context(), tables: map[string]*entry{}, logs: map[string]*entry{}}

	sess.handle(api.ClientMsg{Type: "nonsense", SubID: "s1"})

	if len(sess.tables) != 0 {
		t.Fatal("an unknown frame started a subscription")
	}
}

func TestRaisingTheLimitSendsAFreshSnapshot(t *testing.T) {
	was := minResyncInterval
	minResyncInterval = time.Millisecond
	t.Cleanup(func() {
		minResyncInterval = was
	})
	ts := awkwardServer(t, &awkward{})
	ctx, conn := openAwkwardFeed(t, ts)

	sendMsg(ctx, t, conn, api.ClientMsg{
		Type:      "subscribe",
		SubID:     "s1",
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "default",
		Limit:     1,
	})
	if first := readMsg(ctx, t, conn); first.Type != "snapshot" {
		t.Fatalf("type = %q, want a snapshot first", first.Type)
	}

	sendMsg(ctx, t, conn, api.ClientMsg{Type: "more", SubID: "s1", Limit: 200})

	again := readMsg(ctx, t, conn)
	if again.Type != "snapshot" {
		t.Fatalf("type = %q, want another snapshot once the limit rose", again.Type)
	}
	if again.Limit != 200 {
		t.Fatalf("limit = %d, want the one that was asked for", again.Limit)
	}
}
