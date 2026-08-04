package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/coder/websocket"
)

func tokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	mgr, _ := testManager(t)
	ts := httptest.NewServer(New(fixed(mgr), testAssets(), testToken).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, ts *httptest.Server, path string, prepare func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if prepare != nil {
		prepare(req)
	}
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestNewTokenIsUnguessableAndFreshEveryRun(t *testing.T) {
	first := NewToken()
	second := NewToken()

	if len(first) < 26 {
		t.Fatalf("token = %q, want at least 26 characters of entropy", first)
	}
	if first == second {
		t.Fatal("two runs produced the same token")
	}
}

func TestBrowserURLCarriesTheToken(t *testing.T) {
	url := BrowserURL("127.0.0.1:34115", "abc")

	if url != "http://127.0.0.1:34115/?token=abc" {
		t.Fatalf("url = %q", url)
	}
}

func TestTokenScriptQuotesTheToken(t *testing.T) {
	script := TokenScript("ab\"c")

	if script != `<script>window.__SPINOZA_TOKEN__="ab\"c";</script>` {
		t.Fatalf("script = %q", script)
	}
}

func TestInjectHeadLeavesADocumentWithoutAHeadAlone(t *testing.T) {
	out := InjectHead([]byte("<html>no head</html>"), "<script></script>")

	if string(out) != "<html>no head</html>" {
		t.Fatalf("out = %q", out)
	}
}

func TestAnAPICallWithoutTheTokenIsRefused(t *testing.T) {
	ts := tokenServer(t)

	res := get(t, ts, "/api/resources", nil)

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; a local process without the token holds no cluster access", res.StatusCode)
	}
}

func TestAWrongTokenIsRefused(t *testing.T) {
	ts := tokenServer(t)

	res := get(t, ts, "/api/resources", func(r *http.Request) {
		r.Header.Set(AuthHeader, "not-the-token")
	})

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}

func TestTheHeaderTokenIsAccepted(t *testing.T) {
	ts := tokenServer(t)

	res := get(t, ts, "/api/resources", func(r *http.Request) {
		r.Header.Set(AuthHeader, testToken)
	})

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestTheQueryTokenIsAcceptedAndKeptInACookie(t *testing.T) {
	ts := tokenServer(t)

	res := get(t, ts, "/?"+AuthParam+"="+testToken, nil)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v, want the token kept so a reload still works", cookies)
	}
	if cookies[0].Name != authCookie || cookies[0].Value != testToken {
		t.Fatalf("cookie = %v", cookies[0])
	}
	if !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %v, want it locked to same-site scripts", cookies[0])
	}
}

func TestTheCookieTokenIsAccepted(t *testing.T) {
	ts := tokenServer(t)

	res := get(t, ts, "/api/resources", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: authCookie, Value: testToken})
	})

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if len(res.Cookies()) != 0 {
		t.Fatalf("cookies = %v, want no re-issue when the browser already holds it", res.Cookies())
	}
}

func TestAServerWithoutATokenAnswersNobody(t *testing.T) {
	mgr, _ := testManager(t)
	ts := httptest.NewServer(New(fixed(mgr), testAssets(), "").Handler())
	t.Cleanup(ts.Close)

	res := get(t, ts, "/api/resources", func(r *http.Request) {
		r.Header.Set(AuthHeader, "")
	})

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; an unset token must not disable the check", res.StatusCode)
	}
}

func TestTheWebsocketNeedsTheToken(t *testing.T) {
	ts := tokenServer(t)

	_, _, err := websocket.Dial(context.Background(), wsURL(ts.URL), nil)

	if err == nil {
		t.Fatal("the socket opened without the token")
	}
}

func TestTheWebsocketTakesTheTokenFromTheQuery(t *testing.T) {
	ts := tokenServer(t)

	conn, _, err := websocket.Dial(context.Background(), wsURL(ts.URL)+"?"+AuthParam+"="+testToken, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.CloseNow()
}

func TestTheExecSocketNeedsTheToken(t *testing.T) {
	ts := tokenServer(t)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/exec?namespace=default&pod=web"

	_, _, err := websocket.Dial(context.Background(), url, nil)

	if err == nil {
		t.Fatal("the exec socket opened without the token")
	}
}

func TestTheIndexCarriesTheTokenAndRefusesToBeFramed(t *testing.T) {
	ts := tokenServer(t)

	res := get(t, ts, "/", func(r *http.Request) {
		r.Header.Set(AuthHeader, testToken)
	})
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !strings.Contains(string(body), `window.__SPINOZA_TOKEN__="`+testToken+`"`) {
		t.Fatalf("body = %s, want the token handed to the page", body)
	}
	if res.Header.Get("Content-Security-Policy") != "frame-ancestors 'none'" {
		t.Fatalf("csp = %q", res.Header.Get("Content-Security-Policy"))
	}
	if res.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("x-frame-options = %q", res.Header.Get("X-Frame-Options"))
	}
}

func TestAStaticAssetAlsoRefusesToBeFramed(t *testing.T) {
	ts := tokenServer(t)

	res := get(t, ts, "/app.js", func(r *http.Request) {
		r.Header.Set(AuthHeader, testToken)
	})
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(body) != "spinoza-bundle" {
		t.Fatalf("body = %s", body)
	}
	if res.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("x-frame-options = %q", res.Header.Get("X-Frame-Options"))
	}
}

func TestTheActionLogNeverCarriesTheToken(t *testing.T) {
	req, err := http.NewRequest(http.MethodDelete,
		"http://127.0.0.1:34115/api/object?resource=secrets&name=db&"+AuthParam+"="+testToken, http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	logged := loggableQuery(req)

	if strings.Contains(logged, testToken) {
		t.Fatalf("log line = %q, want the token left out of the terminal", logged)
	}
	if !strings.Contains(logged, "resource=secrets") {
		t.Fatalf("log line = %q, want the object it acted on", logged)
	}
	if !strings.Contains(logged, "name=db") {
		t.Fatalf("log line = %q, want the object it acted on", logged)
	}
}

func TestAReadIsNotLoggedButAWriteIs(t *testing.T) {
	cases := map[string]bool{
		http.MethodGet:    false,
		http.MethodHead:   false,
		http.MethodPost:   true,
		http.MethodPut:    true,
		http.MethodPatch:  true,
		http.MethodDelete: true,
	}
	for method, want := range cases {
		t.Run(method, func(t *testing.T) {
			if mutating(method) != want {
				t.Fatalf("mutating(%s) = %v", method, mutating(method))
			}
		})
	}
}

func TestAMissingIndexIsAnInternalFault(t *testing.T) {
	mgr, _ := testManager(t)
	empty := fstest.MapFS{}
	ts := httptest.NewServer(New(fixed(mgr), empty, testToken).Handler())
	t.Cleanup(ts.Close)

	res := get(t, ts, "/", func(r *http.Request) {
		r.Header.Set(AuthHeader, testToken)
	})

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
}
