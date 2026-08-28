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
	"github.com/coder/websocket/wsjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// Wraps the listener so writes can be made to fail after n frames.
type breaking struct {
	writes     atomic.Int64
	failAt     atomic.Int64
	accepted   atomic.Int64
	brokenUpTo atomic.Int64
	failing    atomic.Bool
}

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

func brokenServer(t *testing.T) (*httptest.Server, *breaking, dynamic.Interface) {
	t.Helper()
	mgr, dyn := testManager(t)
	state := &breaking{}
	srv := New(fixed(mgr), testAssets(), testToken)
	ts := httptest.NewUnstartedServer(authed(srv.Handler()))
	ts.Listener = &breakingListener{Listener: ts.Listener, state: state}
	ts.Start()
	t.Cleanup(ts.Close)
	return ts, state, dyn
}

// Waits out the opening frames; breaking earlier breaks a greeting instead.
func openBrokenFeed(t *testing.T, ts *httptest.Server) (context.Context, *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	greeted(ctx, t, conn)
	return ctx, conn
}

func greeted(ctx context.Context, t *testing.T, conn *websocket.Conn) {
	t.Helper()
	seen := map[string]bool{}
	for len(seen) < len(openingFrames) {
		msg := readAnyMsg(ctx, t, conn)
		if !aboutTheConnection(msg.Type) {
			t.Fatalf("type = %q, want the frames a feed opens with", msg.Type)
		}
		seen[msg.Type] = true
	}
}

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

	socket.after(0)
	_, err := dyn.Resource(depGVR).
		Namespace("default").
		Create(ctx, newDeployment("default", "api"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	stillServing(t, ts)
}

func waitForWrites(t *testing.T, socket *breaking, wanted int64) {
	t.Helper()
	start := socket.wrote()
	deadline := time.After(15 * time.Second)
	for socket.wrote() < start+wanted {
		select {
		case <-deadline:
			t.Fatalf(
				"the server is at %d writes, waiting for %d more after %d",
				socket.wrote(), wanted, start,
			)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestALogLineThatCannotBeWrittenDoesNotTakeTheServerDown(t *testing.T) {
	ts, socket, _ := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)

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

func TestThePodCountFrameFailingAfterALogBatchIsSurvived(t *testing.T) {
	ts, socket, _ := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)

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

	socket.after(0)
	sendMsg(ctx, t, conn, api.ClientMsg{Type: "more", SubID: "s1", Limit: 200})
	waitForWrites(t, socket, 1)

	stillServing(t, ts)
}

func TestAFeedWhoseWindowGoesAwayMidPauseStops(t *testing.T) {
	ts, _, _ := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)
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
	sendMsg(ctx, t, conn, api.ClientMsg{Type: "more", SubID: "s1", Limit: 300})
	time.Sleep(50 * time.Millisecond)
	_ = conn.CloseNow()

	stillServing(t, ts)
}

// Write counts start once a feed is open, so opening frames must be known.
func TestAFeedIsSentItsOpeningFramesAndThenNothing(t *testing.T) {
	ts, socket, _ := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)
	settled := socket.wrote()

	quiet, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	var extra api.ServerMsg
	err := wsjson.Read(quiet, conn, &extra)

	if err == nil {
		t.Fatalf("a feed that asked for nothing was sent a %q frame", extra.Type)
	}
	if socket.wrote() != settled {
		t.Fatalf("the server wrote %d frames at a feed that asked for nothing", socket.wrote()-settled)
	}
}

// The first write a socket refuses is the last one the server attempts on it.
func TestNothingMoreIsWrittenToASocketThatHasGone(t *testing.T) {
	ts, socket, _ := brokenServer(t)
	ctx, conn := openBrokenFeed(t, ts)
	socket.after(0)
	sendMsg(ctx, t, conn, api.ClientMsg{
		Type: "subscribe", SubID: "s1", Group: "apps", Version: "v1", Resource: "deployments",
	})
	waitForWrites(t, socket, 1)
	refused := socket.wrote()

	_ = wsjson.Write(ctx, conn, api.ClientMsg{
		Type: "subscribe", SubID: "s2", Group: "apps", Version: "v1", Resource: "deployments",
	})
	time.Sleep(250 * time.Millisecond)

	if socket.wrote() != refused {
		t.Fatalf("the server made %d more writes into a socket that had gone", socket.wrote()-refused)
	}
	stillServing(t, ts)
}
