package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// A connection that works and then does not is the one thing a test cannot ask
// a real socket for. breaking wraps the listener, so every frame the server
// writes goes through a Write that can be told to start failing — after a
// given number of frames, so a test can let some through first.
type breaking struct {
	writes   atomic.Int64
	failAt   atomic.Int64
	accepted atomic.Int64
	// brokenUpTo is the last connection the break applies to. Anything opened
	// afterwards is a fresh connection to a server that is still running, which
	// is how a test asks whether it survived.
	brokenUpTo atomic.Int64
	failing    atomic.Bool
}

// after tells the sockets that are open now to fail once n more frames have
// gone out.
func (b *breaking) after(n int64) {
	b.failAt.Store(b.writes.Load() + n)
	b.brokenUpTo.Store(b.accepted.Load())
	b.failing.Store(true)
}

func (b *breaking) refuse(conn int64) bool {
	if !b.failing.Load() {
		return false
	}
	if conn > b.brokenUpTo.Load() {
		return false
	}
	return b.writes.Load() > b.failAt.Load()
}

func (b *breaking) wrote() int64 {
	return b.writes.Load()
}

type breakingConn struct {
	net.Conn

	state *breaking
	id    int64
}

func (c *breakingConn) Write(p []byte) (int, error) {
	c.state.writes.Add(1)
	if c.state.refuse(c.id) {
		return 0, errors.New("the connection went away")
	}
	return c.Conn.Write(p)
}

type breakingListener struct {
	net.Listener

	state *breaking
}

func (l *breakingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &breakingConn{Conn: conn, state: l.state, id: l.state.accepted.Add(1)}, nil
}

// brokenServer hands back the server, the switch that breaks its socket, and
// the dynamic client behind it, so a test can make the cluster change
// underneath a subscription that can no longer be written to.
func brokenServer(t *testing.T) (*httptest.Server, *breaking, dynamic.Interface) {
	t.Helper()
	return brokenServerResyncing(t, defaultResyncInterval)
}

func brokenServerResyncing(t *testing.T, resync time.Duration) (*httptest.Server, *breaking, dynamic.Interface) {
	t.Helper()
	mgr, dyn := testManager(t)
	state := &breaking{}
	srv := New(fixed(mgr), testAssets(), testToken)
	srv.resyncEvery = resync
	ts := httptest.NewUnstartedServer(authed(srv.Handler()))
	ts.Listener = &breakingListener{Listener: ts.Listener, state: state}
	ts.Start()
	t.Cleanup(ts.Close)
	return ts, state, dyn
}

func openBrokenFeed(t *testing.T, ts *httptest.Server) (context.Context, *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return ctx, conn
}

// stillServing proves the server is alive after a socket failed under it, which
// is the property that matters: one window going away must not take the process
// with it.
func stillServing(t *testing.T, ts *httptest.Server) {
	t.Helper()
	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/version", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the server stopped serving after a socket failed: %d %s", resp.StatusCode, body)
	}
}

func TestASnapshotThatCannotBeWrittenDoesNotTakeTheServerDown(t *testing.T) {
	ts, socket, _ := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)
	readAnyMsg(ctx, t, conn)

	socket.after(0)
	sendMsg(ctx, t, conn, api.ClientMsg{
		Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "deployments",
	})
	waitForWrites(t, socket, 1)

	stillServing(t, ts)
}

func TestAnUpdateThatCannotBeWrittenDoesNotTakeTheServerDown(t *testing.T) {
	ts, socket, dyn := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)
	readAnyMsg(ctx, t, conn)
	sendMsg(ctx, t, conn, api.ClientMsg{
		Type:      "subscribe",
		SubID:     "s1",
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "default",
	})
	if first := readMsg(ctx, t, conn); first.Type != "snapshot" {
		t.Fatalf("type = %q, want a snapshot before the socket breaks", first.Type)
	}

	// The snapshot got through; the update that follows will not.
	socket.after(0)
	_, err := dyn.Resource(depGVR).
		Namespace("default").
		Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	stillServing(t, ts)
}

func waitForWrites(t *testing.T, socket *breaking, n int64) {
	t.Helper()
	start := socket.wrote()
	deadline := time.After(5 * time.Second)
	for socket.wrote() < start+n {
		select {
		case <-deadline:
			t.Fatalf("the server never tried to write again (%d frames)", socket.wrote())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestALogLineThatCannotBeWrittenDoesNotTakeTheServerDown(t *testing.T) {
	ts, socket, _ := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)
	readAnyMsg(ctx, t, conn)

	socket.after(0)
	sendMsg(ctx, t, conn, api.ClientMsg{
		Type:      "logs-subscribe",
		SubID:     "logs",
		Namespace: "default",
		Name:      "web",
		Container: "app",
		Follow:    true,
	})
	waitForWrites(t, socket, 1)

	stillServing(t, ts)
}

// The batch of lines got through and the frame that follows it — how many pods
// are being read — did not. The relay has to stop rather than carry on writing
// into a socket that is gone.
func TestThePodCountFrameFailingAfterALogBatchIsSurvived(t *testing.T) {
	ts, socket, _ := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)
	readAnyMsg(ctx, t, conn)

	// Let one more frame through, then break: the log-open frame lands and what
	// comes after it does not.
	socket.after(1)
	sendMsg(ctx, t, conn, api.ClientMsg{
		Type:      "logs-subscribe",
		SubID:     "logs",
		Namespace: "default",
		Name:      "web",
		Container: "app",
		Follow:    true,
	})
	waitForWrites(t, socket, 2)

	stillServing(t, ts)
}

func TestAResyncThatCannotBeWrittenDoesNotTakeTheServerDown(t *testing.T) {
	ts, socket, _ := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)
	readAnyMsg(ctx, t, conn)
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

	// Raising the limit asks for a fresh snapshot, which is the write that fails.
	socket.after(0)
	sendMsg(ctx, t, conn, api.ClientMsg{Type: "more", SubID: "s1", Limit: 200})
	waitForWrites(t, socket, 1)

	stillServing(t, ts)
}

// A feed whose window goes away in the middle of the pause between resyncs has
// nothing left to write to, and has to give up rather than wait out the pause.
func TestAFeedWhoseWindowGoesAwayMidPauseStops(t *testing.T) {
	ts, _, _ := brokenServerResyncing(t, 2*time.Second)
	ctx, conn := openBrokenFeed(t, ts)
	readAnyMsg(ctx, t, conn)
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
	readMsg(ctx, t, conn)
	// A second resync lands inside the pause the first one started.
	sendMsg(ctx, t, conn, api.ClientMsg{Type: "more", SubID: "s1", Limit: 300})
	time.Sleep(50 * time.Millisecond)
	_ = conn.CloseNow()

	stillServing(t, ts)
}
