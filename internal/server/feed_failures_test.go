package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type awkward struct {
	Backend

	subscribeErr error
	logsErr      error
	selectorErr  error
	objectYAML   string
	hold         chan struct{}
}

func (a *awkward) wait() {
	if a.hold != nil {
		<-a.hold
	}
}

func (a *awkward) Subscribe(
	ctx context.Context,
	group, version, resource, namespace string,
	limit int,
	filters []api.RowFilter,
) (*resources.Subscription, error) {
	a.wait()
	if a.subscribeErr != nil {
		return nil, a.subscribeErr
	}
	return a.Backend.Subscribe(ctx, group, version, resource, namespace, limit, filters)
}

func (a *awkward) Object(ctx context.Context, ref api.ObjectRef) (api.ObjectDetail, error) {
	if a.objectYAML != "" {
		return api.ObjectDetail{Kind: "Pod", Name: ref.Name, Namespace: ref.Namespace, YAML: a.objectYAML}, nil
	}
	return a.Backend.Object(ctx, ref)
}

func (a *awkward) Logs(ctx context.Context, req logs.Request) (*logs.Stream, error) {
	a.wait()
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
	srv := New(&brokenCluster{stubCluster: cluster, backend: broken}, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

type brokenCluster struct {
	*stubCluster

	backend Backend
}

func (b *brokenCluster) Manager(string) Backend {
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

func TestASubscriptionReplacedWhileItWasBeingBuiltIsDropped(t *testing.T) {
	broken := &awkward{hold: make(chan struct{})}
	ts := awkwardServer(t, broken)
	ctx, conn := openAwkwardFeed(t, ts)
	subscribe := api.ClientMsg{
		Type:      "subscribe",
		SubID:     "s1",
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "default",
	}

	sendMsg(ctx, t, conn, subscribe)
	sendMsg(ctx, t, conn, subscribe)
	time.Sleep(50 * time.Millisecond)
	close(broken.hold)

	if first := readMsg(ctx, t, conn); first.Type != "snapshot" {
		t.Fatalf("type = %q, want the surviving subscription's snapshot", first.Type)
	}
	quiet, stop := context.WithTimeout(ctx, 300*time.Millisecond)
	defer stop()
	var extra api.ServerMsg
	if err := wsjson.Read(quiet, conn, &extra); err == nil && extra.Type == "snapshot" {
		t.Fatal("both builds delivered a snapshot; the replaced one was not dropped")
	}
}

func TestALogStreamReplacedWhileItWasBeingOpenedIsDropped(t *testing.T) {
	broken := &awkward{hold: make(chan struct{})}
	ts := awkwardServer(t, broken)
	ctx, conn := openAwkwardFeed(t, ts)
	subscribe := api.ClientMsg{
		Type:      "logs-subscribe",
		SubID:     "logs",
		Namespace: "prod",
		Name:      "web",
		Container: "app",
	}

	sendMsg(ctx, t, conn, subscribe)
	sendMsg(ctx, t, conn, subscribe)
	time.Sleep(50 * time.Millisecond)
	close(broken.hold)

	opened := 0
	quiet, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()
	for {
		var msg api.ServerMsg
		if err := wsjson.Read(quiet, conn, &msg); err != nil {
			break
		}
		if msg.Type == "log-open" {
			opened++
		}
	}
	if opened > 1 {
		t.Fatalf("%d streams were opened; the replaced one was not dropped", opened)
	}
}

func TestComparingAnObjectWhoseYamlWillNotParse(t *testing.T) {
	ts := awkwardServer(t, &awkward{objectYAML: "\tnot: [yaml"})

	resp, body := doRequest(
		t,
		http.MethodGet,
		ts.URL+"/api/compare?version=v1&resource=pods&namespace=prod&name=web&against=p-mk1",
		nil,
	)

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("a document that will not parse compared fine: %s", body)
	}
}

func TestComparingAnObjectRawSkipsTheParsing(t *testing.T) {
	ts := awkwardServer(t, &awkward{objectYAML: "\tnot: [yaml"})

	resp, _ := doRequest(
		t,
		http.MethodGet,
		ts.URL+"/api/compare?version=v1&resource=pods&namespace=prod&name=web&against=p-mk1&raw=true",
		nil,
	)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want raw to hand the document over untouched", resp.StatusCode)
	}
}

func TestABatchForAReplacedSubscriptionIsNotWritten(t *testing.T) {
	sess := &wsSession{ctx: t.Context(), tables: map[string]*entry{}, logs: map[string]*entry{}}
	gen := sess.claim(tables, "s1")
	sess.claim(tables, "s1")

	events := make(chan resources.Event)
	close(events)
	wrote := sess.writeBatch("s1", gen, resources.Event{Kind: "added"}, events)

	if wrote {
		t.Fatal("a batch was written for a subscription that had been replaced")
	}
}

func TestDrainingStopsWhenTheEventsAreClosed(t *testing.T) {
	events := make(chan resources.Event)
	close(events)

	done := make(chan struct{})
	go func() {
		drainEvents(events)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("draining a closed channel never returned")
	}
}

func TestALineHeldBackFromTheLastBatchIsHandedOverFirst(t *testing.T) {
	sess := &wsSession{ctx: t.Context(), tables: map[string]*entry{}, logs: map[string]*entry{}}
	held := &logs.Line{Pod: "web-1", Text: "the line that ended the last batch"}

	line, step := sess.nextLine(nil, held, nil)

	if step != relayLine {
		t.Fatalf("step = %v, want the held line handed over", step)
	}
	if line != held {
		t.Fatalf("line = %+v, want the one that was held back", line)
	}
}

func (b *brokenCluster) ID() string {
	return "https://broken:6443"
}

func (b *brokenCluster) Open(api.ContextRef) (string, error) {
	return b.ID(), nil
}

func (b *brokenCluster) Activate(string) error {
	return nil
}

func (b *brokenCluster) Opened() []api.OpenCluster {
	return []api.OpenCluster{{ID: b.ID(), Active: true, Protection: api.ProtectionUnknown}}
}

func (b *brokenCluster) Close(string) error {
	return nil
}
