package server

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

func TestAuthorizationWatcherUsesTheDefaultInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		authn, err := auth.New(t.Context(), auth.Config{Mode: auth.ModeNone})
		if err != nil {
			t.Fatalf("building authentication: %v", err)
		}
		srv := New(nil, nil, "")
		srv.UseClusterAuth(ClusterAuth{Authenticator: authn})
		req := liveRequest(t, "alice")
		ctx, cancel := context.WithCancel(t.Context())
		who, known := auth.IdentityFrom(req.Context())
		done := make(chan struct{})
		started := time.Now()
		go func() {
			srv.watchAuthorization(ctx, cancel, req, who, known, who.User, nil)
			close(done)
		}()

		<-ctx.Done()
		<-done

		if elapsed := time.Since(started); elapsed != defaultAuthorizationCheckInterval {
			t.Fatalf("authorization check ran after %v, want %v", elapsed, defaultAuthorizationCheckInterval)
		}
	})
}
