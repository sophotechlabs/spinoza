package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

func TestAuthorizationWatcherKeepsAValidProxyIdentityConnected(t *testing.T) {
	secret := strings.Repeat("p", 32)
	authn, err := auth.New(t.Context(), auth.Config{
		Mode: auth.ModeProxy,
		Proxy: auth.ProxyConfig{
			SharedSecret: []byte(secret),
		},
	})
	if err != nil {
		t.Fatalf("build authenticator: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	req.Header.Set(auth.DefaultUserHeader, "alice")
	req.Header.Set(auth.DefaultGroupsHeader, "platform")
	req.Header.Set(auth.DefaultProxyAuthHeader, secret)
	who, known := authn.Identify(httptest.NewRecorder(), req)
	if !known {
		t.Fatal("the proxy identity was not accepted for the test")
	}

	srv := New(nil, nil, "")
	srv.UseClusterAuth(ClusterAuth{Authenticator: authn})
	srv.authEvery = time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		srv.watchAuthorization(ctx, cancel, req, who, known, who.User)
		close(done)
	}()

	select {
	case <-ctx.Done():
		t.Fatal("a still-valid proxy identity was disconnected")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the authorization watcher ignored cancellation")
	}
}
