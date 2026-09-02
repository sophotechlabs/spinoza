package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type changingFleet struct {
	*fleet

	openedMu    sync.Mutex
	openedCalls int
}

func (held *changingFleet) Opened() []api.OpenCluster {
	held.openedMu.Lock()
	held.openedCalls++
	call := held.openedCalls
	held.openedMu.Unlock()
	open := held.fleet.Opened()
	if call == 1 {
		return open
	}
	return open[:1]
}

func TestFleetCheckPageReportsAClusterThatClosesDuringTheRequestAsAConflict(t *testing.T) {
	base := &fleet{
		held: []api.OpenCluster{
			{ID: mk1, Context: "p-mk1", Active: true},
			{ID: mk2, Context: "p-mk2"},
		},
		active: mk1,
		backends: map[string]Backend{
			mk1: &listing{},
			mk2: &listing{},
		},
	}
	srv := New(&changingFleet{fleet: base}, testAssets(), testToken)
	cursor := encodeFleetCheckCursor("limits-missing", map[string]string{mk1: "one", mk2: "two"})
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/checks/findings/fleet?check=limits-missing&after="+url.QueryEscape(cursor),
		http.NoBody,
	)
	recorded := httptest.NewRecorder()

	srv.fleetCheckPage(recorded, request)

	if recorded.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", recorded.Code, recorded.Body.String())
	}
	if !strings.Contains(recorded.Body.String(), mk2) {
		t.Fatalf("body = %q, want the cluster that closed named", recorded.Body.String())
	}
}
