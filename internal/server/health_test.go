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

// unreachable is a backend whose cluster can be taken away and given back while
// a test watches what the windows are told.
type flaky struct {
	Backend

	mu    sync.Mutex
	err   error
	asks  int
	heard *reach.Sink
}

// Reach stands in for the transport under a real client: a test can tell the
// server what a request came back with, without one having been made.
func (u *flaky) Reach() *reach.Sink {
	return u.heard
}

func (u *flaky) Ping(context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.asks++
	return u.err
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

// flakyServer hands back a server that probes the cluster often enough for a
// test to watch it change its mind.
func flakyServer(t *testing.T, backend *flaky) *httptest.Server {
	t.Helper()
	return flakyServerEvery(t, backend, 20*time.Millisecond)
}

func flakyServerEvery(t *testing.T, backend *flaky, every time.Duration) *httptest.Server {
	t.Helper()
	mgr, _ := testManager(t)
	backend.Backend = mgr
	srv := New(&brokenCluster{stubCluster: fixed(mgr), backend: backend}, testAssets(), testToken)
	srv.pingEvery = every
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

// nextHealth reads until the cluster says something about itself.
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

// awaitHealth reads until the cluster is said to be in the state a test is
// waiting for.
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
	backend := &flaky{}
	ts := flakyServer(t, backend)
	ctx, conn := openAwkwardFeed(t, ts)
	nextHealth(ctx, t, conn)

	// Let several probes go by with nothing changing.
	for backend.asked() < 4 {
		time.Sleep(10 * time.Millisecond)
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
	ts := flakyServer(t, backend)
	ctx, conn := openAwkwardFeed(t, ts)
	nextHealth(ctx, t, conn)
	for backend.asked() < 2 {
		time.Sleep(10 * time.Millisecond)
	}

	_ = conn.Close(websocket.StatusNormalClosure, "done")

	// Give the prober time to notice, then check it has stopped asking.
	time.Sleep(150 * time.Millisecond)
	settled := backend.asked()
	time.Sleep(150 * time.Millisecond)
	if backend.asked() != settled {
		t.Fatalf("asked %d then %d; the prober kept going with nobody watching", settled, backend.asked())
	}
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

func TestSwitchingClusterForgetsWhatWasKnown(t *testing.T) {
	mgr, _ := testManager(t)
	server := New(fixed(mgr), testAssets(), testToken)
	server.recordHealth(api.ClusterHealth{Type: "cluster", Reachable: false, Reason: "gone"})

	server.forgetHealth()

	if !server.clusterHealth().Reachable {
		t.Fatal("what was known about the last cluster was said about the next one")
	}
}

// The ping is for a cluster nobody is asking anything of. When a user does run
// into an outage, the window should hear about it then, not up to a ping later.
const noPingWillArriveInTime = 10 * time.Minute

func TestAWindowHearsAboutAFailedRequestWithoutWaitingForThePing(t *testing.T) {
	backend := &flaky{heard: reach.New()}
	ts := flakyServerEvery(t, backend, noPingWillArriveInTime)
	ctx, conn := openAwkwardFeed(t, ts)
	if first := nextHealth(ctx, t, conn); !first.Reachable {
		t.Fatalf("health = %+v, want it reachable to begin with", first)
	}

	backend.heard.Saw(errors.New("dial tcp 10.0.0.1:6443: connect: connection refused"))

	gone := awaitHealth(ctx, t, conn, false)
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
	awaitHealth(ctx, t, conn, false)

	backend.heard.Saw(nil)

	back := awaitHealth(ctx, t, conn, true)
	if back.Reason != "" {
		t.Fatalf("reason = %q, want it forgotten once the cluster answered", back.Reason)
	}
}

// A backend that reports nothing is what a manager built without a real client
// looks like. The prober has to carry on asking.
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

// noCluster is a spinoza that has not managed to connect to anything yet.
type noCluster struct {
	Cluster
}

func (noCluster) Manager() Backend {
	return nil
}

func TestAServerWithNoClusterHasNothingToReport(t *testing.T) {
	srv := New(noCluster{}, testAssets(), testToken)

	health := srv.reachHealth()

	if !health.Reachable {
		t.Fatalf("health = %+v, want the benefit of the doubt before a cluster is picked", health)
	}
}
