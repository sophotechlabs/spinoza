package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/store"
)

func auditIdentityServer(t *testing.T) (*Server, *heldHistory) {
	t.Helper()
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	held := &heldHistory{}
	srv.UseHistory(t.Context(), held)
	return srv, held
}

func auditIdentityRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, pathSession, http.NoBody)
}

func TestEachIdentityEventIsRecordedWithItsOwnVerbAndOutcome(t *testing.T) {
	who := auth.Identity{User: "alice@example.com", Role: auth.RoleViewer}
	cases := []struct {
		name    string
		made    func(srv *Server, r *http.Request)
		verb    string
		outcome string
		message string
	}{
		{
			name:    "signing in is allowed and says so",
			made:    func(srv *Server, r *http.Request) { srv.RecordIdentity(r, AuditSignedIn, who) },
			verb:    AuditSignedIn,
			outcome: api.HistoryDone,
		},
		{
			name:    "signing out is allowed and says so",
			made:    func(srv *Server, r *http.Request) { srv.RecordIdentity(r, AuditSignedOut, who) },
			verb:    AuditSignedOut,
			outcome: api.HistoryDone,
		},
		{
			name: "a session that ran out is refused with its reason",
			made: func(srv *Server, r *http.Request) {
				srv.RecordIdentityRefused(r, AuditSessionExpired, who, "the session ran out")
			},
			verb:    AuditSessionExpired,
			outcome: api.HistoryRefused,
			message: "the session ran out",
		},
		{
			name: "a role that is not enough is refused with its reason",
			made: func(srv *Server, r *http.Request) {
				srv.RecordIdentityRefused(r, AuditRoleRefused, who, "your role here is viewer; this needs admin")
			},
			verb:    AuditRoleRefused,
			outcome: api.HistoryRefused,
			message: "your role here is viewer; this needs admin",
		},
		{
			name: "acting as somebody else is refused with its reason",
			made: func(srv *Server, r *http.Request) {
				srv.RecordIdentityRefused(r, AuditActingRefused, who, "spinoza does not act as somebody else")
			},
			verb:    AuditActingRefused,
			outcome: api.HistoryRefused,
			message: "spinoza does not act as somebody else",
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			srv, held := auditIdentityServer(t)

			one.made(srv, auditIdentityRequest())

			entry := held.only(t)
			if entry.Verb != one.verb {
				t.Errorf("verb = %q, want %q", entry.Verb, one.verb)
			}
			if entry.Outcome != one.outcome {
				t.Errorf("outcome = %q, want %q", entry.Outcome, one.outcome)
			}
			if entry.Message != one.message {
				t.Errorf("message = %q, want %q", entry.Message, one.message)
			}
			if entry.Actor != "alice@example.com" {
				t.Errorf("actor = %q, want the person it was about", entry.Actor)
			}
		})
	}
}

func TestAnIdentityEventNamesNoObject(t *testing.T) {
	srv, held := auditIdentityServer(t)

	srv.RecordIdentity(auditIdentityRequest(), AuditSignedIn, auth.Identity{User: "alice@example.com"})

	entry := held.only(t)
	empty := map[string]string{
		"group":    entry.Group,
		"version":  entry.Version,
		"resource": entry.Resource,
		"name":     entry.Name,
		"kind":     entry.Kind,
	}
	for field, value := range empty {
		if value != "" {
			t.Errorf("%s = %q, want nothing: an identity event changes no object", field, value)
		}
	}
	if !entry.At.Equal(recordedAt) {
		t.Errorf("at = %v, want the moment it happened", entry.At)
	}
}

func TestAnIdentityEventWithNobodySignedInIsRecordedAsAnonymous(t *testing.T) {
	srv, held := auditIdentityServer(t)

	srv.RecordIdentityRefused(auditIdentityRequest(), AuditSessionExpired, auth.Identity{}, "the session ran out")

	if actor := held.only(t).Actor; actor != "anonymous" {
		t.Fatalf("actor = %q, want anonymous", actor)
	}
}

func TestAnIdentityEventLandsOnTheClusterTheRequestIsOn(t *testing.T) {
	srv, held := auditIdentityServer(t)

	srv.RecordIdentity(auditIdentityRequest(), AuditSignedIn, auth.Identity{User: "alice@example.com"})

	if cluster := held.only(t).Cluster; cluster != stubClusterID {
		t.Fatalf("cluster = %q, want %q", cluster, stubClusterID)
	}
}

func TestAnIdentityEventWithoutAHistoryIsDroppedQuietly(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)

	srv.RecordIdentity(auditIdentityRequest(), AuditSignedIn, auth.Identity{User: "alice@example.com"})
	srv.RecordIdentityRefused(auditIdentityRequest(), AuditRoleRefused, auth.Identity{}, "no role at all")
}

func TestAnIdentityEventTheStoreRefusedIsNotFatal(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	held := &heldHistory{recordErr: errors.New("the history file is read-only")}
	srv.UseHistory(t.Context(), held)

	srv.RecordIdentity(auditIdentityRequest(), AuditSignedIn, auth.Identity{User: "alice@example.com"})

	if recorded := held.recorded(); len(recorded) != 0 {
		t.Fatalf("the refused entry was kept anyway: %+v", recorded)
	}
}

func TestIdentityEventsCountTowardsAuditRetention(t *testing.T) {
	srv, held := auditIdentityServer(t)
	srv.auditPruneEvery = 2

	srv.RecordIdentity(auditIdentityRequest(), AuditSignedIn, auth.Identity{User: "alice@example.com"})
	if len(held.auditTrims()) != 1 {
		t.Fatal("audit retention ran before the write boundary")
	}
	srv.RecordIdentity(auditIdentityRequest(), AuditSignedOut, auth.Identity{User: "alice@example.com"})

	if len(held.auditTrims()) != 2 {
		t.Fatalf("audit prune calls = %d, want startup and the write boundary", len(held.auditTrims()))
	}
}

func TestAnIdentityEventReadsBackThroughTheHistoryView(t *testing.T) {
	held := &heldHistory{page: store.Page{Entries: []store.Entry{{
		ID:      4,
		At:      recordedAt,
		Verb:    AuditRoleRefused,
		Actor:   "alice@example.com",
		Outcome: api.HistoryRefused,
		Message: "your role here is viewer; this needs admin",
	}}}}
	ts := pastServer(t, held)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	got := readBack(t, body)
	if len(got.Entries) != 1 {
		t.Fatalf("entries = %d, want the identity row", len(got.Entries))
	}
	row := got.Entries[0]
	if row.Verb != AuditRoleRefused || row.Outcome != api.HistoryRefused {
		t.Fatalf("the identity row came back as %+v", row)
	}
	if row.Name != "" || row.Resource != "" {
		t.Fatalf("the identity row named an object: %+v", row)
	}
}
