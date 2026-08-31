package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testSessions(t *testing.T, secure bool) *sessions {
	t.Helper()
	return newSessions([]byte("a-test-secret"), time.Hour, 24*time.Hour, secure)
}

func decoded(held *sessions, value string) (Identity, bool) {
	got, ok := held.decode(value)
	return got.who, ok
}

func withCookie(t *testing.T, recorded *httptest.ResponseRecorder) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody)
	for _, cookie := range recorded.Result().Cookies() {
		req.AddCookie(cookie)
	}
	return req
}

func TestASessionComesBackTheWayItWasIssued(t *testing.T) {
	held := testSessions(t, false)
	recorded := httptest.NewRecorder()

	err := held.issue(recorded, Identity{
		User:    "alice@example.com",
		Groups:  []string{"platform", "sre"},
		Role:    RoleEditor,
		Session: "session-7",
	}, time.Now())
	if err != nil {
		t.Fatalf("issuing the session: %v", err)
	}

	got, ok := held.read(withCookie(t, recorded))
	who := got.who
	if !ok {
		t.Fatal("the cookie spinoza set did not read back")
	}
	if who.User != "alice@example.com" || who.Role != RoleEditor || who.Session != "session-7" {
		t.Fatalf("identity = %+v, want the one that was issued", who)
	}
	if strings.Join(who.Groups, ",") != "platform,sre" {
		t.Fatalf("groups = %v, want both", who.Groups)
	}
}

func TestASessionCookieIsLockedDown(t *testing.T) {
	recorded := httptest.NewRecorder()

	err := testSessions(t, true).issue(recorded, Identity{User: "alice"}, time.Now())
	if err != nil {
		t.Fatalf("issuing the session: %v", err)
	}

	cookie := recorded.Result().Cookies()[0]
	if !cookie.HttpOnly {
		t.Fatal("the session cookie is readable from javascript")
	}
	if !cookie.Secure {
		t.Fatal("a session for an https site was allowed over plain http")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("samesite = %v, want lax so the login redirect still carries it", cookie.SameSite)
	}
}

func TestAnEditedCookieIsNotASession(t *testing.T) {
	held := testSessions(t, false)
	recorded := httptest.NewRecorder()
	_ = held.issue(recorded, Identity{User: "alice", Role: RoleViewer}, time.Now())
	cookie := recorded.Result().Cookies()[0]

	body, signature, _ := strings.Cut(cookie.Value, ".")
	forged := body[:len(body)-1] + "X." + signature

	if _, ok := decoded(held, forged); ok {
		t.Fatal("a cookie whose payload was edited still read as a session")
	}
	if _, ok := decoded(held, body); ok {
		t.Fatal("a cookie with no signature at all read as a session")
	}
	if _, ok := decoded(held, "not-base64.zzzz"); ok {
		t.Fatal("a cookie that is not even base64 read as a session")
	}
}

func TestACookieSignedWithAnotherKeyIsRefused(t *testing.T) {
	mine := testSessions(t, false)
	theirs := newSessions([]byte("another-secret"), time.Hour, 24*time.Hour, false)
	value, err := theirs.encode(Identity{User: "mallory", Role: RoleAdmin}, time.Now())
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}

	if _, ok := decoded(mine, value); ok {
		t.Fatal("a cookie from another spinoza was accepted")
	}
}

func TestAnExpiredSessionIsGone(t *testing.T) {
	held := testSessions(t, false)
	value, err := held.encode(Identity{User: "alice", Role: RoleViewer}, time.Now())
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	held.now = func() time.Time { return time.Now().Add(2 * time.Hour) }

	if _, ok := decoded(held, value); ok {
		t.Fatal("a session past its expiry still read as valid")
	}
}

func TestASessionNamingNobodyIsNotASession(t *testing.T) {
	held := testSessions(t, false)
	value, err := held.encode(Identity{Role: RoleAdmin}, time.Now())
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}

	if _, ok := decoded(held, value); ok {
		t.Fatal("a cookie with no username read as somebody")
	}
}

func TestTooManyGroupsToFitInACookieIsSaidOutLoud(t *testing.T) {
	held := testSessions(t, false)
	groups := make([]string, 0, 400)
	for i := range 400 {
		groups = append(groups, strings.Repeat("g", 20)+string(rune('a'+i%26)))
	}

	err := held.issue(httptest.NewRecorder(), Identity{User: "alice", Groups: groups}, time.Now())
	if err == nil {
		t.Fatal("a session far past the cookie limit was issued anyway")
	}
	if !strings.Contains(err.Error(), "more groups than a cookie holds") {
		t.Fatalf("error = %q, want it to name the reason", err.Error())
	}
}

func TestClearingASessionExpiresTheCookie(t *testing.T) {
	recorded := httptest.NewRecorder()

	testSessions(t, false).clear(recorded)

	cookie := recorded.Result().Cookies()[0]
	if cookie.Name != SessionCookie {
		t.Fatalf("cookie = %q, want %q", cookie.Name, SessionCookie)
	}
	if cookie.MaxAge >= 0 {
		t.Fatalf("maxAge = %d, want it negative so the browser drops it", cookie.MaxAge)
	}
}

func TestNoCookieAtAllIsNoSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/overview", http.NoBody)

	if _, ok := testSessions(t, false).read(req); ok {
		t.Fatal("a request with no cookie read as signed in")
	}
}

func TestAStashedFlowComesBackAndCanBeDropped(t *testing.T) {
	held := testSessions(t, false)
	recorded := httptest.NewRecorder()

	err := held.stash(recorded, flowCookie, flowState{State: "s", Verifier: "v", Nonce: "n", Return: "/checks"}, time.Minute)
	if err != nil {
		t.Fatalf("stashing: %v", err)
	}

	var back flowState
	if !held.unstash(withCookie(t, recorded), &back) {
		t.Fatal("the login state spinoza stashed did not come back")
	}
	if back.Return != "/checks" || back.Verifier != "v" {
		t.Fatalf("flow = %+v, want what was stashed", back)
	}

	dropped := httptest.NewRecorder()
	held.drop(dropped, flowCookie)
	if dropped.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatal("dropping the login state left the cookie in place")
	}
}

func TestAnEditedFlowCookieIsIgnored(t *testing.T) {
	held := testSessions(t, false)
	recorded := httptest.NewRecorder()
	_ = held.stash(recorded, flowCookie, flowState{State: "s"}, time.Minute)
	cookie := recorded.Result().Cookies()[0]

	cases := map[string]string{
		"no signature":    strings.Split(cookie.Value, ".")[0],
		"wrong signature": strings.Split(cookie.Value, ".")[0] + ".AAAA",
		"not base64":      "%%%.AAAA",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/callback", http.NoBody)
			req.AddCookie(&http.Cookie{Name: flowCookie, Value: value})
			var back flowState
			if held.unstash(req, &back) {
				t.Fatal("a tampered login state was accepted")
			}
		})
	}
}

func TestAMissingFlowCookieIsIgnored(t *testing.T) {
	var back flowState
	req := httptest.NewRequest(http.MethodGet, "/auth/callback", http.NoBody)

	if testSessions(t, false).unstash(req, &back) {
		t.Fatal("a request with no login state read as having one")
	}
}

func TestEverySecretAndSessionIDIsItsOwn(t *testing.T) {
	secrets := map[string]bool{}
	ids := map[string]bool{}
	for range 32 {
		secrets[string(NewSecret())] = true
		ids[newSessionID()] = true
	}

	if len(secrets) != 32 {
		t.Fatalf("%d distinct secrets out of 32", len(secrets))
	}
	if len(ids) != 32 {
		t.Fatalf("%d distinct session ids out of 32", len(ids))
	}
}

func TestASessionPastHalfItsLifeIsRenewedRatherThanExpiringUnderSomebody(t *testing.T) {
	held := testSessions(t, false)
	now := time.Now()
	held.now = func() time.Time { return now }

	if held.halfSpent(now.Add(time.Hour)) {
		t.Fatal("a session with a full hour left was renewed")
	}
	if !held.halfSpent(now.Add(time.Minute)) {
		t.Fatal("a session about to expire was left alone")
	}
}

func TestASessionStopsBeingRenewedOnceItHasRunItsCourse(t *testing.T) {
	now := time.Now()
	held := newSessions([]byte("a-test-secret"), time.Hour, 6*time.Hour, false)
	held.now = func() time.Time { return now }

	if !held.renewable(now.Add(-time.Hour)) {
		t.Fatal("a sign-in an hour old was already past its cap")
	}
	if held.renewable(now.Add(-7 * time.Hour)) {
		t.Fatal("a sign-in past the cap was still being renewed")
	}
}

func TestARenewedSessionKeepsTheInstantItWasSignedInAt(t *testing.T) {
	now := time.Now()
	held := testSessions(t, false)
	held.now = func() time.Time { return now }
	issued := now.Add(-3 * time.Hour)

	value, err := held.encode(Identity{User: "alice", Role: RoleViewer}, issued)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}

	got, ok := held.decode(value)
	if !ok {
		t.Fatal("the session did not read back")
	}
	if got.issued.Unix() != issued.Unix() {
		t.Fatalf("issued = %s, want the original %s", got.issued, issued)
	}
	if got.expires.Unix() != now.Add(time.Hour).Unix() {
		t.Fatalf("expires = %s, want an hour from now", got.expires)
	}
}
