package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/reach"
)

type flaky struct {
	Backend

	mu     sync.Mutex
	err    error
	asks   int
	heard  *reach.Sink
	probed chan struct{}
}

func (u *flaky) Reach() *reach.Sink {
	return u.heard
}

func (u *flaky) Ping(context.Context) error {
	u.mu.Lock()
	u.asks++
	err := u.err
	u.mu.Unlock()
	if u.probed != nil {
		select {
		case u.probed <- struct{}{}:
		default:
		}
	}
	return err
}

func (u *flaky) breaks(err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.err = err
}

func (u *flaky) asked() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.asks
}

func flakyServer(t *testing.T, backend *flaky) *httptest.Server {
	t.Helper()
	return flakyServerEvery(t, backend, 20*time.Millisecond)
}

func flakyServerEvery(t *testing.T, backend *flaky, every time.Duration) *httptest.Server {
	t.Helper()
	ts, _ := flakyServerAndInstance(t, backend, every)
	return ts
}

func flakyServerAndInstance(t *testing.T, backend *flaky, every time.Duration) (*httptest.Server, *Server) {
	t.Helper()
	mgr, _ := testManager(t)
	backend.Backend = mgr
	srv := New(&brokenCluster{stubCluster: fixed(mgr), backend: backend}, testAssets(), testToken)
	srv.pingEvery = every
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts, srv
}

func nextHealth(ctx context.Context, t *testing.T, conn *websocket.Conn) api.ServerMsg {
	t.Helper()
	for range 60 {
		msg := readAnyMsg(ctx, t, conn)
		if msg.Type == "cluster" {
			return msg
		}
	}
	t.Fatal("nothing was ever said about the cluster")
	return api.ServerMsg{}
}

func awaitHealth(ctx context.Context, t *testing.T, conn *websocket.Conn, want bool) api.ServerMsg {
	t.Helper()
	for range 60 {
		msg := nextHealth(ctx, t, conn)
		if msg.Reachable == want {
			return msg
		}
	}
	t.Fatalf("the cluster was never said to be reachable=%v", want)
	return api.ServerMsg{}
}

func awaitWobble(ctx context.Context, t *testing.T, conn *websocket.Conn) api.ServerMsg {
	t.Helper()
	for range 60 {
		msg := nextHealth(ctx, t, conn)
		if msg.Wobbling {
			return msg
		}
	}
	t.Fatal("the cluster was never said to be wobbling")
	return api.ServerMsg{}
}

func awaitAnswering(ctx context.Context, t *testing.T, conn *websocket.Conn) api.ServerMsg {
	t.Helper()
	for range 60 {
		msg := nextHealth(ctx, t, conn)
		if !msg.Reachable {
			continue
		}
		if msg.Wobbling {
			continue
		}
		return msg
	}
	t.Fatal("the cluster was never said to answer again")
	return api.ServerMsg{}
}

func TestAWindowIsToldTheClusterAnswers(t *testing.T) {
	ts := flakyServer(t, &flaky{})
	ctx, conn := openAwkwardFeed(t, ts)

	health := nextHealth(ctx, t, conn)

	if !health.Reachable {
		t.Fatalf("health = %+v, want a cluster that answers", health)
	}
}

func TestAWindowIsToldWhenTheClusterStopsAnswering(t *testing.T) {
	backend := &flaky{}
	ts := flakyServer(t, backend)
	ctx, conn := openAwkwardFeed(t, ts)
	if first := nextHealth(ctx, t, conn); !first.Reachable {
		t.Fatalf("health = %+v, want it reachable to begin with", first)
	}

	backend.breaks(errors.New("dial tcp 10.0.0.1:6443: connect: connection refused"))

	gone := awaitHealth(ctx, t, conn, false)
	if gone.Reachable {
		t.Fatalf("health = %+v, want the window told the cluster went away", gone)
	}
	if gone.Reason == "" {
		t.Fatal("the window was told the cluster is unreachable without being told why")
	}
	if !strings.Contains(gone.Reason, "connection refused") {
		t.Fatalf("reason = %q, want what the cluster said", gone.Reason)
	}
}

func TestAWindowIsToldWhenTheClusterComesBack(t *testing.T) {
	backend := &flaky{err: errors.New("connection refused")}
	ts := flakyServer(t, backend)
	ctx, conn := openAwkwardFeed(t, ts)
	awaitHealth(ctx, t, conn, false)

	backend.breaks(nil)

	back := awaitHealth(ctx, t, conn, true)
	if !back.Reachable {
		t.Fatalf("health = %+v, want the window told the cluster answers again", back)
	}
}

func TestAnUnchangedAnswerIsNotRepeated(t *testing.T) {
	backend := &flaky{probed: make(chan struct{}, 8)}
	ts := flakyServer(t, backend)
	ctx, conn := openAwkwardFeed(t, ts)
	nextHealth(ctx, t, conn)

	for range 4 {
		select {
		case <-backend.probed:
		case <-time.After(time.Second):
			t.Fatal("the cluster was not probed four times")
		}
	}

	quiet, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	for {
		var msg api.ServerMsg
		if err := wsjson.Read(quiet, conn, &msg); err != nil {
			return
		}
		if msg.Type == "cluster" {
			t.Fatal("the same answer was sent again")
		}
	}
}

func TestTheProberStopsWhenTheLastWindowGoes(t *testing.T) {
	backend := &flaky{}
	ts, srv := flakyServerAndInstance(t, backend, 20*time.Millisecond)
	ctx, conn := openAwkwardFeed(t, ts)
	nextHealth(ctx, t, conn)
	waitForServer(t, func() bool { return backend.asked() >= 2 }, "the prober never repeated its probe")

	_ = conn.Close(websocket.StatusNormalClosure, "done")
	waitForServer(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		return !srv.watching
	}, "the prober kept watching after the last window closed")

	before := backend.asked()
	nextCtx, nextConn := openAwkwardFeed(t, ts)
	nextHealth(nextCtx, t, nextConn)
	waitForServer(t, func() bool { return backend.asked() > before }, "the prober did not restart for a new window")

	_ = nextConn.Close(websocket.StatusNormalClosure, "done")
}

func TestWhatCountsAsReachable(t *testing.T) {
	if health := healthOf(nil); !health.Reachable {
		t.Fatalf("health = %+v, want a cluster that answered", health)
	}
	broken := healthOf(errors.New("connection refused"))
	if broken.Reachable {
		t.Fatalf("health = %+v, want unreachable", broken)
	}
	if broken.Reason != "connection refused" {
		t.Fatalf("reason = %q, want what went wrong", broken.Reason)
	}
	if broken.Type != "cluster" {
		t.Fatalf("type = %q", broken.Type)
	}
}

func TestNothingIsClaimedBeforeTheFirstProbe(t *testing.T) {
	mgr, _ := testManager(t)
	server := New(fixed(mgr), testAssets(), testToken)

	health := server.clusterHealth()

	if !health.Reachable {
		t.Fatal("a window was told the cluster is down before anything was asked")
	}
}

const noPingWillArriveInTime = 10 * time.Minute

func TestAWindowHearsAboutAFailedRequestWithoutWaitingForThePing(t *testing.T) {
	backend := &flaky{heard: reach.New()}
	ts := flakyServerEvery(t, backend, noPingWillArriveInTime)
	ctx, conn := openAwkwardFeed(t, ts)
	if first := nextHealth(ctx, t, conn); !first.Reachable {
		t.Fatalf("health = %+v, want it reachable to begin with", first)
	}

	backend.heard.Saw(errors.New("dial tcp 10.0.0.1:6443: connect: connection refused"))

	gone := awaitWobble(ctx, t, conn)
	if !strings.Contains(gone.Reason, "connection refused") {
		t.Fatalf("reason = %q, want what the request ran into", gone.Reason)
	}
}

func TestAWindowHearsTheClusterAnsweringAgainFromTheNextRequestThatWorks(t *testing.T) {
	backend := &flaky{heard: reach.New()}
	ts := flakyServerEvery(t, backend, noPingWillArriveInTime)
	ctx, conn := openAwkwardFeed(t, ts)
	nextHealth(ctx, t, conn)
	backend.heard.Saw(errors.New("connection refused"))
	awaitWobble(ctx, t, conn)

	backend.heard.Saw(nil)

	back := awaitAnswering(ctx, t, conn)
	if back.Reason != "" {
		t.Fatalf("reason = %q, want it forgotten once the cluster answered", back.Reason)
	}
}

func TestAClusterWithNothingToReportIsStillProbed(t *testing.T) {
	backend := &flaky{}
	ts := flakyServer(t, backend)
	ctx, conn := openAwkwardFeed(t, ts)
	nextHealth(ctx, t, conn)

	backend.breaks(errors.New("connection refused"))

	gone := awaitHealth(ctx, t, conn, false)
	if gone.Reachable {
		t.Fatalf("health = %+v, want the ping to have found it", gone)
	}
}

type noCluster struct {
	Cluster
}

func (noCluster) Manager(string) Backend {
	return nil
}

func TestAServerWithNoClusterHasNothingToReport(t *testing.T) {
	srv := New(noCluster{}, testAssets(), testToken)

	health := srv.clusterHealth()

	if !health.Reachable {
		t.Fatalf("health = %+v, want the benefit of the doubt before a cluster is picked", health)
	}
}

func (noCluster) ID() string {
	return ""
}

func (noCluster) Open(api.ContextRef) (string, error) {
	return "", errors.New("this stub connects to nothing")
}

func (noCluster) Activate(string) error {
	return errors.New("this stub connects to nothing")
}

func (noCluster) Opened() []api.OpenCluster {
	return nil
}

func (noCluster) Close(string) error {
	return errors.New("this stub connects to nothing")
}
