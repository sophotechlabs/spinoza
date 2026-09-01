package auth

import (
	"testing"
	"time"
)

func TestARevokedSessionStaysRevokedUntilItWouldHaveExpired(t *testing.T) {
	now := time.Now()
	held := newRevocations(time.Hour)
	held.now = func() time.Time { return now }

	held.revoke("session-7")

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
	held.revoke("session-7")

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

	held.revoke("")

	if len(held.gone) != 0 {
		t.Fatalf("revocations = %v, want none", held.gone)
	}
}

func TestRevokingSweepsWhatHasAlreadyExpired(t *testing.T) {
	now := time.Now()
	held := newRevocations(time.Hour)
	held.now = func() time.Time { return now }
	held.revoke("old")

	now = now.Add(2 * time.Hour)
	held.revoke("new")

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
