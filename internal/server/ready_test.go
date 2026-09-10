package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type readyCase struct {
	name    string
	served  bool
	catalog []api.Category
	syncing bool
	past    History
	want    api.Ready
}

func readyServerFor(t *testing.T, one readyCase) *Server {
	t.Helper()
	backend := &stubCatalog{catalog: api.ResourceCatalog{Categories: one.catalog}}
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
	if one.served {
		srv.UseClusterAuth(ClusterAuth{})
	}
	if one.past != nil {
		srv.UseHistory(t.Context(), one.past)
	}
	if one.syncing {
		srv.track(&wsSession{tables: map[string]*entry{"main": {gen: 1}}, logs: map[string]*entry{}})
	}
	return srv
}

func readyWorkloads() []api.Category {
	return []api.Category{{Name: "Workloads", Resources: []api.ResourceDescriptor{deploymentDesc()}}}
}

func TestReadyNamesEachThingItIsStillWaitingFor(t *testing.T) {
	cases := []readyCase{
		{
			name:    "a pod that has only just started",
			served:  true,
			syncing: true,
			want:    api.Ready{Waiting: []string{readyNoCatalog, readyNoInformers, readyNoStore}},
		},
		{
			name:    "the catalog is read and nothing else is",
			served:  true,
			catalog: readyWorkloads(),
			syncing: true,
			want:    api.Ready{Discovery: true, Waiting: []string{readyNoInformers, readyNoStore}},
		},
		{
			name:    "the catalog is read and the caches have filled",
			served:  true,
			catalog: readyWorkloads(),
			want:    api.Ready{Discovery: true, Informers: true, Waiting: []string{readyNoStore}},
		},
		{
			name:    "the store is open but will not record",
			served:  true,
			catalog: readyWorkloads(),
			past:    &heldHistory{reason: "spinoza is not recording history: /data/history.db is read-only"},
			want:    api.Ready{Discovery: true, Informers: true, Waiting: []string{readyNoStore}},
		},
		{
			name:    "all three answer",
			served:  true,
			catalog: readyWorkloads(),
			past:    &heldHistory{},
			want:    api.Ready{Ready: true, Discovery: true, Informers: true, Store: true},
		},
		{
			name: "a laptop with nothing to gate on",
			want: api.Ready{Ready: true, Discovery: true, Informers: true, Store: true},
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			got := readyServerFor(t, one).Ready()

			if !reflect.DeepEqual(got, one.want) {
				t.Fatalf("ready = %+v, want %+v", got, one.want)
			}
		})
	}
}

func TestReadySaysItIsShuttingDownOnceTheDrainHasStarted(t *testing.T) {
	srv := readyServerFor(t, readyCase{served: true, catalog: readyWorkloads(), past: &heldHistory{}})
	if !srv.Ready().Ready {
		t.Fatal("this server was not ready before the drain, so the test proves nothing")
	}

	srv.Drain(t.Context())
	got := srv.Ready()

	if got.Ready {
		t.Fatal("readiness still said yes while the process was shutting down")
	}
	if !slices.Contains(got.Waiting, drainReason) {
		t.Fatalf("waiting = %v, want it to say the process is shutting down", got.Waiting)
	}
	if !got.Discovery || !got.Informers || !got.Store {
		t.Fatalf("ready = %+v, want the three checks to keep reporting what they found", got)
	}
}

func TestTheReadyEndpointAnswers503WithWhatItIsWaitingFor(t *testing.T) {
	srv := readyServerFor(t, readyCase{served: true, syncing: true})
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/readyz", http.NoBody)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 so the endpoints controller holds the pod back: %s", resp.StatusCode, body)
	}
	var state api.Ready
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	want := []string{readyNoCatalog, readyNoInformers, readyNoStore}
	if !reflect.DeepEqual(state.Waiting, want) {
		t.Fatalf("waiting = %v, want %v", state.Waiting, want)
	}
}

func TestTheReadyEndpointAnswers200OnceEverythingAnswers(t *testing.T) {
	srv := readyServerFor(t, readyCase{served: true, catalog: readyWorkloads(), past: &heldHistory{}})
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/readyz", http.NoBody)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	var state api.Ready
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if len(state.Waiting) != 0 {
		t.Fatalf("waiting = %v, want nothing left to wait for", state.Waiting)
	}
}

func TestReadyIgnoresAFeedThatHasFinishedSubscribing(t *testing.T) {
	srv := readyServerFor(t, readyCase{served: true, catalog: readyWorkloads(), past: &heldHistory{}})
	srv.track(&wsSession{
		tables: map[string]*entry{"main": {gen: 1, resource: &readyStopper{}}},
		logs:   map[string]*entry{},
	})

	got := srv.Ready()

	if !got.Informers {
		t.Fatalf("ready = %+v, want a subscription that has synced to leave readiness alone", got)
	}
}

type readyStopper struct{}

func (readyStopper) Close() {}

func TestOnceItIsReadyANewInformerDoesNotTakeItOutOfRotation(t *testing.T) {
	backend := &stubCatalog{catalog: api.ResourceCatalog{Categories: readyWorkloads()}}
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
	srv.UseClusterAuth(ClusterAuth{})
	srv.UseHistory(t.Context(), &heldHistory{})
	if !srv.Ready().Ready {
		t.Fatal("this server was not ready to begin with, so the test proves nothing")
	}

	backend.syncing = true
	got := srv.Ready()

	if !got.Ready {
		t.Fatal("a cache filling after startup took the pod out of rotation")
	}
	if len(got.Waiting) != 0 {
		t.Fatalf("waiting = %v, want nothing", got.Waiting)
	}
}

func TestAServerWithNoClusterOpenIsNotReady(t *testing.T) {
	srv := New(&stubBackendCluster{}, testAssets(), testToken)
	srv.UseClusterAuth(ClusterAuth{})
	srv.UseHistory(t.Context(), &heldHistory{})

	got := srv.Ready()

	if got.Ready || got.Discovery || got.Informers {
		t.Fatalf("ready = %+v, want nothing answering while no cluster is open", got)
	}
	if !slices.Contains(got.Waiting, readyNoCatalog) || !slices.Contains(got.Waiting, readyNoInformers) {
		t.Fatalf("waiting = %v, want the catalog and the informers named", got.Waiting)
	}
}

func TestACacheStillFillingAtStartupKeepsThePodOutOfRotation(t *testing.T) {
	backend := &stubCatalog{catalog: api.ResourceCatalog{Categories: readyWorkloads()}, syncing: true}
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
	srv.UseClusterAuth(ClusterAuth{})
	srv.UseHistory(t.Context(), &heldHistory{})

	got := srv.Ready()

	if got.Informers || got.Ready {
		t.Fatalf("ready = %+v, want a filling cache to hold readiness back", got)
	}
	if !slices.Contains(got.Waiting, readyNoInformers) {
		t.Fatalf("waiting = %v, want the informers named", got.Waiting)
	}
}

func TestALogStreamStillSubscribingKeepsThePodOutOfRotation(t *testing.T) {
	srv := readyServerFor(t, readyCase{served: true, catalog: readyWorkloads(), past: &heldHistory{}})
	srv.track(&wsSession{
		tables: map[string]*entry{},
		logs:   map[string]*entry{"main": {gen: 1}},
	})

	got := srv.Ready()

	if got.Informers || got.Ready {
		t.Fatalf("ready = %+v, want a log stream that has not synced to hold readiness back", got)
	}
}
