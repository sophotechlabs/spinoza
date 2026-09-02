package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

func liveRequest(t *testing.T, session string) *http.Request {
	t.Helper()
	return liveRequestAt(t, "/ws", session)
}

func liveRequestAt(t *testing.T, path, session string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
	who := auth.Identity{User: session, Role: auth.RoleViewer, Session: session}
	return req.WithContext(auth.WithIdentity(req.Context(), who))
}

func TestLiveConnectionBudgetsAreGlobalAndIdentityScoped(t *testing.T) {
	srv := New(nil, nil, "")
	srv.liveLimit = 2
	srv.identityLimit = 1

	releaseAlice, ok := srv.claimLiveConnection(liveRequest(t, "alice"))
	if !ok {
		t.Fatal("alice's first connection was refused")
	}
	defer releaseAlice()
	if release, claimed := srv.claimLiveConnection(liveRequest(t, "alice")); claimed {
		release()
		t.Fatal("one identity exceeded its connection budget")
	}

	releaseBob, ok := srv.claimLiveConnection(liveRequest(t, "bob"))
	if !ok {
		t.Fatal("bob's connection was refused while the global budget had room")
	}
	if release, claimed := srv.claimLiveConnection(liveRequest(t, "carol")); claimed {
		release()
		t.Fatal("the global connection budget was exceeded")
	}

	releaseBob()
	releaseCarol, ok := srv.claimLiveConnection(liveRequest(t, "carol"))
	if !ok {
		t.Fatal("a released slot was not reusable")
	}
	releaseCarol()
	releaseCarol()
}

func TestLiveConnectionIdentityFallsBackToTheSessionID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		Session: "session-7",
		Role:    auth.RoleViewer,
	}))

	if got := liveIdentity(req); got != "session-7" {
		t.Fatalf("identity = %q, want the session id", got)
	}
}

func TestAuthorizationWatcherEndsAConnectionWhoseIdentityChanged(t *testing.T) {
	authn, err := auth.New(t.Context(), auth.Config{Mode: auth.ModeNone})
	if err != nil {
		t.Fatalf("building authentication: %v", err)
	}
	srv := New(nil, nil, "")
	srv.UseClusterAuth(ClusterAuth{Authenticator: authn})
	srv.authEvery = time.Millisecond
	req := liveRequest(t, "alice")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	who, known := auth.IdentityFrom(req.Context())
	done := make(chan struct{})
	go func() {
		srv.watchAuthorization(ctx, cancel, req, who, known, liveIdentity(req))
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("the invalid live identity was not disconnected")
	}
	<-done
}

func TestEveryLiveEndpointRefusesAnUpgradeWhenTheGlobalBudgetIsFull(t *testing.T) {
	srv := New(nil, nil, "")
	srv.liveLimit = 1
	srv.UseLocalShell(func(uint16, uint16) (LocalShell, error) {
		t.Fatal("the local shell opener ran without a connection slot")
		return nil, errors.New("the local shell opener ran")
	})
	release, ok := srv.claimLiveConnection(liveRequest(t, "alice"))
	if !ok {
		t.Fatal("the setup connection was refused")
	}
	defer release()

	for _, tc := range []struct {
		name string
		path string
		call http.HandlerFunc
	}{
		{name: "feed", path: "/ws", call: srv.handleWS},
		{name: "exec", path: "/api/exec?namespace=default&pod=web", call: srv.handleExec},
		{name: "node shell", path: "/api/nodeshell?node=worker-1", call: srv.handleNodeShell},
		{name: "local shell", path: "/api/shell", call: srv.handleLocalShell},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorded := httptest.NewRecorder()
			tc.call(recorded, liveRequestAt(t, tc.path, "bob"))
			if recorded.Code != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want %d", recorded.Code, http.StatusTooManyRequests)
			}
		})
	}
}
