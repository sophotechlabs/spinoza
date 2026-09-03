package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/logs"
)

const ghost = "https://p-mk3:6443"

func twoBackendServer(t *testing.T, first, second Backend) (*Server, *httptest.Server) {
	t.Helper()
	srv, _ := twoClusters(t, first, second)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return srv, ts
}

func subscribeOn(ctx context.Context, t *testing.T, conn *websocket.Conn, subID, cluster string) {
	t.Helper()
	sendClient(ctx, t, conn, api.ClientMsg{
		Type:      "subscribe",
		SubID:     subID,
		Cluster:   cluster,
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "default",
	})
}

func snapshotNames(ctx context.Context, t *testing.T, conn *websocket.Conn) []string {
	t.Helper()
	msg := readMsg(ctx, t, conn)
	if msg.Type != "snapshot" {
		t.Fatalf("type = %q, want a snapshot: %s", msg.Type, msg.Message)
	}
	names := make([]string, 0, len(msg.Rows))
	for _, row := range msg.Rows {
		names = append(names, row.Name)
	}
	return names
}

func TestASubscriptionGoesToTheClusterItNames(t *testing.T) {
	here, _ := testManager(t, newDeployment("default", "here"))
	there, _ := testManager(t, newDeployment("default", "there"))
	_, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)

	subscribeOn(ctx, t, conn, "s1", mk2)

	names := snapshotNames(ctx, t, conn)
	if len(names) != 1 || names[0] != "there" {
		t.Fatalf("rows = %v, want what the named cluster holds", names)
	}
}

func TestASubscriptionThatNamesNoClusterGoesToTheActiveOne(t *testing.T) {
	here, _ := testManager(t, newDeployment("default", "here"))
	there, _ := testManager(t, newDeployment("default", "there"))
	_, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)

	subscribeOn(ctx, t, conn, "s1", "")

	names := snapshotNames(ctx, t, conn)
	if len(names) != 1 || names[0] != "here" {
		t.Fatalf("rows = %v, want the active cluster's", names)
	}
}

func TestASubscriptionToAClusterThatIsNotOpenIsReported(t *testing.T) {
	here, _ := testManager(t, newDeployment("default", "here"))
	there, _ := testManager(t)
	_, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)

	subscribeOn(ctx, t, conn, "s1", ghost)

	msg := readMsg(ctx, t, conn)
	if msg.Type != msgError {
		t.Fatalf("type = %q, want an error frame", msg.Type)
	}
	if !strings.Contains(msg.Message, ghost) {
		t.Fatalf("message = %q, want the cluster that is not open named", msg.Message)
	}
}

func TestAFeedSurvivesASubscriptionToAClusterThatIsNotOpen(t *testing.T) {
	here, _ := testManager(t, newDeployment("default", "here"))
	there, _ := testManager(t)
	_, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)
	subscribeOn(ctx, t, conn, "s1", ghost)
	readMsg(ctx, t, conn)

	subscribeOn(ctx, t, conn, "s2", mk1)

	if names := snapshotNames(ctx, t, conn); len(names) != 1 {
		t.Fatalf("rows = %v, want the feed still usable after a bad cluster", names)
	}
}

func TestASubscriptionToAClusterThatIsNotOpenIsNotRemembered(t *testing.T) {
	here, _ := testManager(t)
	there, _ := testManager(t)
	srv, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)

	subscribeOn(ctx, t, conn, "s1", ghost)
	readMsg(ctx, t, conn)

	awaitNothingHeld(t, onlySession(t, srv), tables)
}

func awaitNothingHeld(t *testing.T, sess *wsSession, which feed) {
	t.Helper()
	for range 200 {
		sess.mu.Lock()
		held := len(sess.entriesOf(which))
		sess.mu.Unlock()
		if held == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("a refused subscription was still held")
}

func onlySession(t *testing.T, srv *Server) *wsSession {
	t.Helper()
	for range 200 {
		open := srv.openSessions()
		if len(open) == 1 {
			return open[0]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no feed was ever registered")
	return nil
}

func TestClosingAClusterStopsOnlyItsSubscriptions(t *testing.T) {
	here, _ := testManager(t, newDeployment("default", "here"))
	there, _ := testManager(t, newDeployment("default", "there"))
	srv, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)
	subscribeOn(ctx, t, conn, "stays", mk1)
	snapshotNames(ctx, t, conn)
	subscribeOn(ctx, t, conn, "goes", mk2)
	snapshotNames(ctx, t, conn)

	doRequest(t, "DELETE", ts.URL+"/api/clusters?cluster="+mk2, nil)

	sess := onlySession(t, srv)
	for range 200 {
		sess.mu.Lock()
		held := len(sess.tables)
		sess.mu.Unlock()
		if held == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if len(sess.tables) != 1 {
		t.Fatalf("holding %d subscriptions, want only the surviving cluster's", len(sess.tables))
	}
	if sess.tables["stays"] == nil {
		t.Fatal("the wrong cluster's subscription was dropped")
	}
}

func TestClosingAClusterLeavesTheFeedReadable(t *testing.T) {
	here, _ := testManager(t, newDeployment("default", "here"))
	there, _ := testManager(t, newDeployment("default", "there"))
	_, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)
	subscribeOn(ctx, t, conn, "goes", mk2)
	snapshotNames(ctx, t, conn)

	doRequest(t, "DELETE", ts.URL+"/api/clusters?cluster="+mk2, nil)
	subscribeOn(ctx, t, conn, "stays", mk1)

	if names := snapshotNames(ctx, t, conn); len(names) != 1 || names[0] != "here" {
		t.Fatalf("rows = %v, want the feed still serving the cluster that stayed", names)
	}
}

func TestTheOpeningFrameNamesTheCluster(t *testing.T) {
	here, _ := testManager(t)
	there, _ := testManager(t)
	_, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)

	msg := readAnyMsg(ctx, t, conn)

	if msg.Type != "context" {
		t.Fatalf("first frame = %q, want the context", msg.Type)
	}
	if msg.Cluster != mk1 {
		t.Fatalf("cluster = %q, want the active cluster named", msg.Cluster)
	}
}

type namedLogs struct {
	Backend

	name string
}

func (b *namedLogs) Logs(context.Context, logs.Request) (*logs.Stream, error) {
	return nil, errors.New("logs from " + b.name)
}

func TestALogStreamGoesToTheClusterItNames(t *testing.T) {
	here, _ := testManager(t)
	there, _ := testManager(t)
	_, ts := twoBackendServer(t,
		&namedLogs{Backend: here, name: "here"},
		&namedLogs{Backend: there, name: "there"})
	ctx, conn := openAwkwardFeed(t, ts)

	sendClient(ctx, t, conn, api.ClientMsg{
		Type: "logs-subscribe", SubID: "l1", Cluster: mk2, Namespace: "default", Name: "web", TailLines: 100,
	})

	msg := readMsg(ctx, t, conn)
	if msg.Message != "logs from there" {
		t.Fatalf("message = %q, want the named cluster asked", msg.Message)
	}
}

func TestALogStreamToAClusterThatIsNotOpenIsReported(t *testing.T) {
	here, _ := testManager(t)
	there, _ := testManager(t)
	srv, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)

	sendClient(ctx, t, conn, api.ClientMsg{
		Type: "logs-subscribe", SubID: "l1", Cluster: ghost, Namespace: "default", Name: "web", TailLines: 100,
	})

	msg := readMsg(ctx, t, conn)
	if msg.Type != msgError || !strings.Contains(msg.Message, ghost) {
		t.Fatalf("message = %q/%q, want the cluster that is not open named", msg.Type, msg.Message)
	}
	awaitNothingHeld(t, onlySession(t, srv), streams)
}

func TestAFeedThatOutlivesTheLastClusterSaysSo(t *testing.T) {
	here, _ := testManager(t)
	there, _ := testManager(t)
	_, ts := twoBackendServer(t, here, there)
	ctx, conn := openAwkwardFeed(t, ts)

	doRequest(t, "DELETE", ts.URL+"/api/clusters?cluster="+mk2, nil)
	doRequest(t, "DELETE", ts.URL+"/api/clusters?cluster="+mk1, nil)
	subscribeOn(ctx, t, conn, "s1", "")

	msg := readMsg(ctx, t, conn)
	if msg.Type != msgError {
		t.Fatalf("type = %q, want an error frame rather than a hang", msg.Type)
	}
	if !strings.Contains(msg.Message, "no cluster") {
		t.Fatalf("message = %q, want it plain that nothing is connected", msg.Message)
	}
}
