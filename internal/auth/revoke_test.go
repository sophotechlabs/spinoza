package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type rememberedRevocations struct {
	mu      sync.Mutex
	held    map[string]time.Time
	written int
	broken  error
}

func (r *rememberedRevocations) Revoke(_ context.Context, session string, until, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.broken != nil {
		return r.broken
	}
	if r.held == nil {
		r.held = map[string]time.Time{}
	}
	r.held[session] = until
	r.written++
	return nil
}

func (r *rememberedRevocations) Revocations(_ context.Context, now time.Time) (map[string]time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.broken != nil {
		return nil, r.broken
	}
	out := map[string]time.Time{}
	for session, until := range r.held {
		if now.Before(until) {
			out[session] = until
		}
	}
	return out, nil
}

func proxyConfigKeeping(kept RevocationStore) Config {
	return Config{
		Mode:          ModeProxy,
		PublicURL:     "https://spinoza.example.com",
		SessionSecret: []byte("the-same-secret-across-restarts--"),
		SessionTTL:    time.Hour,
		SessionMaxAge: 72 * time.Hour,
		Proxy:         ProxyConfig{SharedSecret: NewSecret()},
		Revocations:   kept,
	}
}

func cookieFor(t *testing.T, held *Authenticator, who Identity) *http.Cookie {
	t.Helper()
	recorded := httptest.NewRecorder()
	if err := held.sessions.issue(recorded, who, held.sessions.now()); err != nil {
		t.Fatalf("issuing a session: %v", err)
	}
	response := recorded.Result()
	defer func() { _ = response.Body.Close() }()
	return response.Cookies()[0]
}

func TestARevocationSurvivesANewAuthenticatorOnTheSameStore(t *testing.T) {
	kept := &rememberedRevocations{}
	before, err := New(t.Context(), proxyConfigKeeping(kept))
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	before.cfg.Mode = ModeOIDC
	who := Identity{User: "alice", Groups: []string{"platform"}, Role: RoleEditor, Session: "session-7"}
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	req.AddCookie(cookieFor(t, before, who))
	if !before.StillValid(req, who) {
		t.Fatal("a current session was not valid")
	}
	before.revoked.revoke(t.Context(), who.Session)
	if kept.written != 1 {
		t.Fatalf("the store saw %d revocations, want the one just made", kept.written)
	}

	after, err := New(t.Context(), proxyConfigKeeping(kept))
	if err != nil {
		t.Fatalf("building the authenticator again: %v", err)
	}
	after.cfg.Mode = ModeOIDC

	if after.StillValid(req, who) {
		t.Fatal("a session revoked before the restart was accepted after it")
	}
	if len(after.revoked.gone) != 1 {
		t.Fatalf("revocations after the restart = %v, want the one read back", after.revoked.gone)
	}
}

func TestARevocationTheStoreCannotWriteStillHoldsForThisProcess(t *testing.T) {
	kept := &rememberedRevocations{broken: errors.New("disk full")}
	held := newRevocations(time.Hour)

	held.kept = kept
	held.revoke(t.Context(), "session-7")

	if !held.revoked("session-7") {
		t.Fatal("a revocation the store refused was dropped for this process too")
	}
}

func TestRevocationsAreReadBackWhenTheStoreIsAttached(t *testing.T) {
	now := time.Now()
	kept := &rememberedRevocations{held: map[string]time.Time{
		"live":    now.Add(time.Hour),
		"expired": now.Add(-time.Hour),
	}}
	held := newRevocations(time.Hour)
	held.now = func() time.Time { return now }

	if err := held.keep(t.Context(), kept); err != nil {
		t.Fatalf("keep: %v", err)
	}

	if !held.revoked("live") {
		t.Fatal("a revocation written before the restart was not honored")
	}
	if held.revoked("expired") {
		t.Fatal("an expired revocation was read back")
	}
}

func TestAStoreThatCannotBeReadDoesNotStopTheAuthenticator(t *testing.T) {
	kept := &rememberedRevocations{broken: errors.New("the file is locked")}

	built, err := New(t.Context(), proxyConfigKeeping(kept))

	if err != nil || built == nil {
		t.Fatalf("New = %v, %v; want the authenticator up with a warning rather than no sign-in at all", built, err)
	}
}

func TestARevokedSessionStaysRevokedUntilItWouldHaveExpired(t *testing.T) {
	now := time.Now()
	held := newRevocations(time.Hour)
	held.now = func() time.Time { return now }

	held.revoke(t.Context(), "session-7")

	if !held.revoked("session-7") {
		t.Fatal("a session the provider ended was still accepted")
	}
	now = now.Add(2 * time.Hour)
	if held.revoked("session-7") {
		t.Fatal("a revocation outlived the session it was about")
	}
}

func TestARevocationEndsAtTheExactExpiryBoundary(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	held := newRevocations(time.Hour)
	held.now = func() time.Time { return now }
	held.revoke(t.Context(), "session-7")

	now = now.Add(time.Hour)

	if held.revoked("session-7") {
		t.Fatal("a revocation survived the exact instant the session expired")
	}
}

func TestNothingIsRevokedByDefault(t *testing.T) {
	held := newRevocations(time.Hour)

	if held.revoked("session-7") {
		t.Fatal("a session nobody ended read as revoked")
	}
	if held.revoked("") {
		t.Fatal("the empty session id read as revoked, which would lock everybody out")
	}
}

func TestAnUnnamedSessionCannotBeRevoked(t *testing.T) {
	held := newRevocations(time.Hour)

	held.revoke(t.Context(), "")

	if len(held.gone) != 0 {
		t.Fatalf("revocations = %v, want none", held.gone)
	}
}

func TestRevokingSweepsWhatHasAlreadyExpired(t *testing.T) {
	now := time.Now()
	held := newRevocations(time.Hour)
	held.now = func() time.Time { return now }
	held.revoke(t.Context(), "old")

	now = now.Add(2 * time.Hour)
	held.revoke(t.Context(), "new")

	if _, still := held.gone["old"]; still {
		t.Fatal("an expired revocation was kept, so the list only ever grows")
	}
	if !held.revoked("new") {
		t.Fatal("the session just revoked was not held")
	}
}

func TestARevocationIsRememberedForAsLongAsASessionCanLive(t *testing.T) {
	authn, err := New(t.Context(), Config{
		Mode:          ModeProxy,
		PublicURL:     "https://spinoza.example.com",
		SessionSecret: NewSecret(),
		SessionTTL:    time.Hour,
		SessionMaxAge: 72 * time.Hour,
		Proxy:         ProxyConfig{SharedSecret: NewSecret()},
	})
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}

	if authn.revoked.within != 72*time.Hour {
		t.Fatalf("revocations are kept %s but a session can live %s, so a revoked one would come back",
			authn.revoked.within, 72*time.Hour)
	}
}

func TestSessionsCanBeRevokedConcurrently(t *testing.T) {
	held := newRevocations(time.Hour)
	const sessions = 64
	var group sync.WaitGroup

	for index := range sessions {
		session := fmt.Sprintf("session-%d", index)
		group.Go(func() {
			held.revoke(t.Context(), session)
			if !held.revoked(session) {
				t.Errorf("%s was accepted after revocation", session)
			}
		})
	}
	group.Wait()

	if len(held.gone) != sessions {
		t.Fatalf("revocations = %d, want %d", len(held.gone), sessions)
	}
}
