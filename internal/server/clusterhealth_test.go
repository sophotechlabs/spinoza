package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/reach"
)

const mk1 = "https://p-mk1:6443"

const mk2 = "https://p-mk2:6443"

type pinger struct {
	notStubbed

	err     error
	sink    *reach.Sink
	entered chan string
	release chan struct{}
	id      string
}

func (p *pinger) Ping(context.Context) error {
	if p.entered != nil {
		p.entered <- p.id
		<-p.release
	}
	return p.err
}

func (p *pinger) Reach() *reach.Sink {
	return p.sink
}

func twoClusters(t *testing.T, first, second Backend) (*Server, *fleet) {
	t.Helper()
	held := &fleet{
		held: []api.OpenCluster{
			{ID: mk1, Context: "p-mk1", Active: true},
			{ID: mk2, Context: "p-mk2"},
		},
		active:   mk1,
		backends: map[string]Backend{mk1: first, mk2: second},
	}
	return New(held, testAssets(), testToken), held
}

func pingUntilSettled(t *testing.T, srv *Server) {
	t.Helper()
	for range missesBeforeUnreachable {
		srv.pingEveryCluster(t.Context())
	}
}

func TestOneClusterGoingDownDoesNotCondemnTheOther(t *testing.T) {
	srv, _ := twoClusters(t,
		&pinger{},
		&pinger{err: errors.New("no route to host")})

	pingUntilSettled(t, srv)

	if !srv.healthOfCluster(mk1).Reachable {
		t.Fatal("the cluster that answered was marked unreachable")
	}
	down := srv.healthOfCluster(mk2)
	if down.Reachable {
		t.Fatal("the cluster that refused was reported as answering")
	}
	if down.Reason != "no route to host" {
		t.Fatalf("reason = %q, want what the cluster said", down.Reason)
	}
	if down.Cluster != mk2 {
		t.Fatalf("cluster = %q, want the frame to name which one it is about", down.Cluster)
	}
}

func TestTheClusterListReportsEachClustersOwnHealth(t *testing.T) {
	srv, _ := twoClusters(t,
		&pinger{},
		&pinger{err: errors.New("i/o timeout")})
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	pingUntilSettled(t, srv)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	byID := map[string]api.OpenCluster{}
	for _, one := range clustersFrom(t, body).Clusters {
		byID[one.ID] = one
	}
	if !byID[mk1].Reachable {
		t.Fatalf("list = %s, want the healthy cluster reachable", body)
	}
	if byID[mk2].Reachable {
		t.Fatalf("list = %s, want the unreachable cluster reported as such", body)
	}
	if byID[mk2].Reason != "i/o timeout" {
		t.Fatalf("reason = %q, want the cluster's own words on the row", byID[mk2].Reason)
	}
}

func TestAClusterNobodyHasPingedGetsTheBenefitOfTheDoubt(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{})

	if !srv.healthOfCluster(mk2).Reachable {
		t.Fatal("a cluster was called unreachable before anything asked it")
	}
}

func TestEveryClusterIsAskedAtOnce(t *testing.T) {
	entered := make(chan string, 2)
	release := make(chan struct{})
	srv, _ := twoClusters(t,
		&pinger{id: mk1, entered: entered, release: release},
		&pinger{id: mk2, entered: entered, release: release})

	done := make(chan struct{})
	go func() {
		srv.pingEveryCluster(t.Context())
		close(done)
	}()

	seen := map[string]bool{}
	for range 2 {
		select {
		case id := <-entered:
			seen[id] = true
		case <-time.After(5 * time.Second):
			t.Fatal("only one cluster was asked; a slow cluster blocks the rest")
		}
	}
	close(release)
	<-done

	if !seen[mk1] || !seen[mk2] {
		t.Fatalf("asked %v, want both before either answered", seen)
	}
}

func TestClosingAClusterForgetsWhatWasKnownAboutIt(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{err: errors.New("gone")})
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	pingUntilSettled(t, srv)
	if srv.healthOfCluster(mk2).Reachable {
		t.Fatal("the fixture did not record the cluster as down")
	}

	doRequest(t, http.MethodDelete, ts.URL+"/api/clusters?cluster="+mk2, nil)

	if !srv.healthOfCluster(mk2).Reachable {
		t.Fatal("what was known about a closed cluster outlived it")
	}
}

func TestABackgroundClustersHealthReachesTheBrowserToo(t *testing.T) {
	srv, held := twoClusters(t, &pinger{}, &pinger{})
	server, client := heldPair(t)
	sess := &wsSession{conn: server, ctx: t.Context()}
	srv.track(sess)

	srv.recordHealthOf(mk2, notAnswering("gone"))

	if held.ID() != mk1 {
		t.Fatalf("active = %q, the fixture moved", held.ID())
	}
	msg := readAnyMsg(t.Context(), t, client)
	if msg.Cluster != mk2 {
		t.Fatalf("frame named %q, want the background cluster; its tab shows the wrong dot otherwise", msg.Cluster)
	}
	if !msg.Wobbling {
		t.Fatalf("frame = %+v, want the background cluster reported as wobbling", msg)
	}
	if msg.Reason != "gone" {
		t.Fatalf("reason = %q, want what the failing request said", msg.Reason)
	}
}

func TestAFeedIsToldAboutEveryOpenClusterWhenItConnects(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{})

	said := srv.healthOfEveryCluster()

	named := map[string]bool{}
	for _, one := range said {
		named[one.Cluster] = true
	}
	if !named[mk1] || !named[mk2] {
		t.Fatalf("opening frames named %v, want every open cluster", named)
	}
}

func TestAFeedWithNoClusterStillHearsSomething(t *testing.T) {
	srv := New(noCluster{}, testAssets(), testToken)

	said := srv.healthOfEveryCluster()

	if len(said) != 1 {
		t.Fatalf("said %d frames with nothing open, want one so the dot has a state", len(said))
	}
}

func TestCancelingTheHealthWatcherClearsItsRunningState(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{})
	srv.mu.Lock()
	srv.watching = true
	srv.mu.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan struct{})
	go func() {
		srv.pingUntilNobodyIsWatching(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the canceled health watcher did not stop")
	}

	srv.mu.Lock()
	watching := srv.watching
	srv.mu.Unlock()
	if watching {
		t.Fatal("the stopped health watcher still claims to be running")
	}
}

func TestAFailedRequestIsHeardWithoutWaitingForAPing(t *testing.T) {
	sink := reach.New()
	srv, _ := twoClusters(t, &pinger{sink: sink}, &pinger{})
	watchers := newSinkWatchers(srv)
	t.Cleanup(watchers.stopAll)

	watchers.follow(t.Context(), []string{mk1, mk2})

	if watchers.watching() != 1 {
		t.Fatalf("watching %d sinks, want only the cluster that has one", watchers.watching())
	}
	sink.Saw(errors.New("connection refused"))
	deadline := time.After(5 * time.Second)
	for !srv.healthOfCluster(mk1).Wobbling {
		select {
		case <-deadline:
			t.Fatal("a failed request never reached the health state")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if srv.healthOfCluster(mk1).Reason != "connection refused" {
		t.Fatalf("reason = %q, want what the failing request said", srv.healthOfCluster(mk1).Reason)
	}
}

func TestASinkWatcherStopsWhenItsClusterCloses(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{sink: reach.New()}, &pinger{sink: reach.New()})
	watchers := newSinkWatchers(srv)
	t.Cleanup(watchers.stopAll)
	watchers.follow(t.Context(), []string{mk1, mk2})

	watchers.follow(t.Context(), []string{mk1})

	if watchers.watching() != 1 {
		t.Fatalf("watching %d sinks after one closed, want 1", watchers.watching())
	}
}

func TestOneMissedPingReadsAsAWobbleNotAnOutage(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{err: errors.New("no route to host")})

	srv.pingEveryCluster(t.Context())

	held := srv.healthOfCluster(mk2)
	if !held.Reachable {
		t.Fatal("a single missed ping was reported as the cluster being gone")
	}
	if !held.Wobbling {
		t.Fatal("a single missed ping was reported as nothing at all")
	}
	if held.Reason != "no route to host" {
		t.Fatalf("reason = %q, want what the cluster said", held.Reason)
	}
}

func TestEnoughMissedPingsIsAnOutage(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{err: errors.New("no route to host")})

	pingUntilSettled(t, srv)

	held := srv.healthOfCluster(mk2)
	if held.Reachable {
		t.Fatalf("%d missed pings still read as answering", missesBeforeUnreachable)
	}
	if held.Wobbling {
		t.Fatal("a settled outage is still reported as a wobble")
	}
}

func TestAClusterThatAnswersAgainStopsWobbling(t *testing.T) {
	flaky := &pinger{err: errors.New("no route to host")}
	srv, _ := twoClusters(t, &pinger{}, flaky)

	srv.pingEveryCluster(t.Context())
	flaky.err = nil
	srv.pingEveryCluster(t.Context())

	held := srv.healthOfCluster(mk2)
	if held.Wobbling || !held.Reachable {
		t.Fatalf("a cluster that answered again reads as %+v", held)
	}
}

func clusterInList(t *testing.T, srv *Server, id string) api.OpenCluster {
	t.Helper()
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/clusters", nil)

	var list api.ClusterList
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
	for _, one := range list.Clusters {
		if one.ID == id {
			return one
		}
	}
	t.Fatalf("no cluster %s in %s", id, body)
	return api.OpenCluster{}
}

func TestTheClusterListSaysAWobblingClusterIsWobbling(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{err: errors.New("no route to host")})

	srv.pingEveryCluster(t.Context())

	one := clusterInList(t, srv, mk2)
	if !one.Reachable {
		t.Fatal("a single missed ping was listed as the cluster being gone")
	}
	if !one.Wobbling {
		t.Fatalf("cluster = %+v, want the wobble the websocket already reports", one)
	}
}

func TestTheClusterListSaysASettledOutageIsNotAWobble(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{err: errors.New("no route to host")})

	pingUntilSettled(t, srv)

	one := clusterInList(t, srv, mk2)
	if one.Reachable {
		t.Fatalf("cluster = %+v, want a settled outage listed as unreachable", one)
	}
	if one.Wobbling {
		t.Fatalf("cluster = %+v, want no wobble mark once the outage settled", one)
	}
}

func TestAFailedRequestSettlesTheSameWayAMissedPingDoes(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{})

	srv.recordHealthOf(mk2, notAnswering("connection refused"))

	first := srv.healthOfCluster(mk2)
	if !first.Reachable || !first.Wobbling {
		t.Fatalf("one failed request reads as %+v, want a wobble", first)
	}
	for range missesBeforeUnreachable {
		srv.recordHealthOf(mk2, notAnswering("connection refused"))
	}
	settled := srv.healthOfCluster(mk2)
	if settled.Reachable || settled.Wobbling {
		t.Fatalf("a cluster that keeps failing reads as %+v, want an outage", settled)
	}
}

func TestPingsAndFailedRequestsShareOneVerdict(t *testing.T) {
	srv, _ := twoClusters(t, &pinger{}, &pinger{err: errors.New("no route to host")})

	srv.pingEveryCluster(t.Context())
	srv.pingEveryCluster(t.Context())
	srv.recordHealthOf(mk2, notAnswering("connection refused"))

	held := srv.healthOfCluster(mk2)
	if held.Reachable || held.Wobbling {
		t.Fatalf("health = %+v, want the third miss to settle whichever writer saw it", held)
	}
}

func TestAnAnsweringRequestClearsTheMissesAPingCounted(t *testing.T) {
	flaky := &pinger{err: errors.New("no route to host")}
	srv, _ := twoClusters(t, &pinger{}, flaky)

	srv.pingEveryCluster(t.Context())
	srv.pingEveryCluster(t.Context())
	srv.recordHealthOf(mk2, answering())
	flaky.err = errors.New("no route to host")
	srv.pingEveryCluster(t.Context())

	held := srv.healthOfCluster(mk2)
	if !held.Reachable || !held.Wobbling {
		t.Fatalf("health = %+v, want the count to have restarted after the cluster answered", held)
	}
}
