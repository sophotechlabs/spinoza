package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubWindow struct {
	mu     sync.Mutex
	shown  int
	hidden int
}

func (w *stubWindow) Show() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.shown++
}

func (w *stubWindow) Hide() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hidden++
}

func (w *stubWindow) counts() (int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.shown, w.hidden
}

func viewServer(t *testing.T, window Window, open BrowserOpener) (*Server, *httptest.Server) {
	t.Helper()
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	if window != nil {
		srv.UseWindow(window)
	}
	if open != nil {
		srv.UseBrowser(open)
	}
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return srv, ts
}

func dialView(t *testing.T, ts *httptest.Server, kind string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?view=" + kind
	conn, _, err := websocket.Dial(t.Context(), url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func decodeView(t *testing.T, body []byte) api.ViewState {
	t.Helper()
	var view api.ViewState
	err := json.Unmarshal(body, &view)
	if err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return view
}

func decodeSwitch(t *testing.T, body []byte) api.ViewSwitch {
	t.Helper()
	var moved api.ViewSwitch
	err := json.Unmarshal(body, &moved)
	if err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return moved
}

func TestABrowserOnlyBuildOffersNoWindow(t *testing.T) {
	_, ts := viewServer(t, nil, nil)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/view", nil)

	if decodeView(t, body).Window {
		t.Fatalf("a build with no window claimed one: %s", body)
	}
}

func TestTheDesktopBuildSaysItHasAWindow(t *testing.T) {
	_, ts := viewServer(t, &stubWindow{}, func() error { return nil })

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/view", nil)

	view := decodeView(t, body)
	if !view.Window || view.Hidden {
		t.Fatalf("view = %+v, want a window that is showing", view)
	}
}

func TestSwitchingToTheBrowserNeedsAWindow(t *testing.T) {
	_, ts := viewServer(t, nil, nil)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/view/browser", nil)

	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
}

func TestSwitchingToTheDesktopNeedsAWindow(t *testing.T) {
	_, ts := viewServer(t, nil, nil)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/view/desktop", nil)

	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}
}

func TestTheWindowHidesOnceTheBrowserArrives(t *testing.T) {
	window := &stubWindow{}
	opened := make(chan struct{}, 1)
	_, ts := viewServer(t, window, func() error {
		opened <- struct{}{}
		return nil
	})
	dialView(t, ts, ViewDesktop)

	done := make(chan api.ViewSwitch, 1)
	go func() {
		_, body := doRequest(t, http.MethodPost, ts.URL+"/api/view/browser", nil)
		done <- decodeSwitch(t, body)
	}()

	select {
	case <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("the browser was never opened")
	}
	dialView(t, ts, ViewBrowser)

	select {
	case moved := <-done:
		if !moved.Switched {
			t.Fatalf("the switch was refused: %s", moved.Reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the switch never finished")
	}
	_, hidden := window.counts()
	if hidden != 1 {
		t.Fatalf("the window was hidden %d times, want once", hidden)
	}
}

func TestTheWindowStaysWhenNoBrowserArrives(t *testing.T) {
	window := &stubWindow{}
	srv, ts := viewServer(t, window, func() error { return nil })
	srv.views.settle(defaultIdleGrace, 50*time.Millisecond)

	_, body := doRequest(t, http.MethodPost, ts.URL+"/api/view/browser", nil)

	moved := decodeSwitch(t, body)
	if moved.Switched {
		t.Fatal("the window was hidden without a browser to switch to")
	}
	if !strings.Contains(moved.Reason, "browser") {
		t.Fatalf("reason = %q", moved.Reason)
	}
	_, hidden := window.counts()
	if hidden != 0 {
		t.Fatalf("the window was hidden %d times, want none", hidden)
	}
}

func TestABrowserAlreadyOpenSwitchesStraightAway(t *testing.T) {
	window := &stubWindow{}
	_, ts := viewServer(t, window, func() error { return nil })
	dialView(t, ts, ViewBrowser)
	waitForServer(t, func() bool { return !srvViews(t, ts).Hidden }, "the server never started")

	_, body := doRequest(t, http.MethodPost, ts.URL+"/api/view/browser", nil)

	if !decodeSwitch(t, body).Switched {
		t.Fatalf("a tab that was already open was not enough: %s", body)
	}
}

func TestABrowserThatWillNotOpenIsReported(t *testing.T) {
	_, ts := viewServer(t, &stubWindow{}, func() error {
		return errors.New("no browser on this machine")
	})

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/view/browser", nil)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "no browser") {
		t.Fatalf("body = %s", body)
	}
}

func TestComingBackShowsTheWindow(t *testing.T) {
	window := &stubWindow{}
	srv, ts := viewServer(t, window, func() error { return nil })
	srv.views.hide()

	_, body := doRequest(t, http.MethodPost, ts.URL+"/api/view/desktop", nil)

	if !decodeSwitch(t, body).Switched {
		t.Fatalf("the switch back was refused: %s", body)
	}
	shown, _ := window.counts()
	if shown != 1 {
		t.Fatalf("the window was shown %d times, want once", shown)
	}
	if srv.views.isHidden() {
		t.Fatal("the server still thinks the window is hidden")
	}
}

func srvViews(t *testing.T, ts *httptest.Server) api.ViewState {
	t.Helper()
	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/view", nil)
	return decodeView(t, body)
}

func TestTheLastViewLeavingStopsTheApp(t *testing.T) {
	stopped := make(chan struct{})
	srv, ts := viewServer(t, nil, nil)
	srv.views.settle(20*time.Millisecond, defaultBrowserAwait)
	srv.UseIdleExit(func() { close(stopped) })
	conn := dialView(t, ts, ViewBrowser)

	_ = conn.CloseNow()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the app kept running with nothing looking at it")
	}
}

func TestAViewComingBackKeepsTheAppAlive(t *testing.T) {
	stopped := make(chan struct{})
	srv, ts := viewServer(t, nil, nil)
	srv.views.settle(300*time.Millisecond, defaultBrowserAwait)
	srv.UseIdleExit(func() { close(stopped) })
	first := dialView(t, ts, ViewBrowser)

	_ = first.CloseNow()
	dialView(t, ts, ViewBrowser)

	select {
	case <-stopped:
		t.Fatal("a reload was treated as the last view leaving")
	case <-time.After(600 * time.Millisecond):
	}
}

func TestAnAppNobodyOpenedIsLeftAlone(t *testing.T) {
	stopped := make(chan struct{})
	srv, _ := viewServer(t, nil, nil)
	srv.views.settle(20*time.Millisecond, defaultBrowserAwait)
	srv.UseIdleExit(func() { close(stopped) })

	srv.views.closed(ViewBrowser)

	select {
	case <-stopped:
		t.Fatal("a server nobody had opened yet stopped itself")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAHiddenWindowDoesNotHoldTheAppOpen(t *testing.T) {
	stopped := make(chan struct{})
	srv, ts := viewServer(t, &stubWindow{}, func() error { return nil })
	srv.views.settle(20*time.Millisecond, defaultBrowserAwait)
	srv.UseIdleExit(func() { close(stopped) })
	dialView(t, ts, ViewDesktop)
	tab := dialView(t, ts, ViewBrowser)
	srv.views.hide()

	_ = tab.CloseNow()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the hidden window kept the app running after the tab closed")
	}
}

func TestAShowingWindowHoldsTheAppOpen(t *testing.T) {
	stopped := make(chan struct{})
	srv, ts := viewServer(t, &stubWindow{}, func() error { return nil })
	srv.views.settle(20*time.Millisecond, defaultBrowserAwait)
	srv.UseIdleExit(func() { close(stopped) })
	dialView(t, ts, ViewDesktop)
	tab := dialView(t, ts, ViewBrowser)

	_ = tab.CloseNow()

	select {
	case <-stopped:
		t.Fatal("the app stopped while its window was still open")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestThePageSaysWhichViewItIs(t *testing.T) {
	if !strings.Contains(ViewScript(ViewDesktop), `"desktop"`) {
		t.Fatalf("script = %s", ViewScript(ViewDesktop))
	}
	_, ts := viewServer(t, nil, nil)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/", nil)

	if !strings.Contains(string(body), `window.__SPINOZA_VIEW__="browser"`) {
		t.Fatalf("the page does not say it is a browser view: %s", body)
	}
}

func TestNoStartRouteInjectsNothing(t *testing.T) {
	if StartScript("", "") != "" {
		t.Fatalf("an unset start route emitted %q", StartScript("", ""))
	}
}

func TestAStartRouteNamesTheViewAndContext(t *testing.T) {
	got := StartScript("traffic", "p-mk1")

	if !strings.Contains(got, `view:"traffic"`) {
		t.Fatalf("script = %q, want the view named", got)
	}
	if !strings.Contains(got, `context:"p-mk1"`) {
		t.Fatalf("script = %q, want the context named", got)
	}
}

func TestAStartRouteCannotCarryScriptOutOfItsQuotes(t *testing.T) {
	got := StartScript(`</script><script>stolen()`, `"; drop()`)

	if strings.Count(got, "</script>") != 1 {
		t.Fatalf("script = %q, want the only closing tag to be its own", got)
	}
}

func TestSettingsCannotCarryScriptOutOfTheirQuotes(t *testing.T) {
	got := SettingsScript(map[string]string{
		"spinoza.columns.v1": `[{"name":"</script><script>stolen()</script>","path":".a"}]`,
	})

	if strings.Count(got, "</script>") != 1 {
		t.Fatalf("script = %q, want the only closing tag to be its own", got)
	}
}

func TestATokenCannotCarryScriptOutOfItsQuotes(t *testing.T) {
	got := TokenScript(`</script><script>stolen()`)

	if strings.Count(got, "</script>") != 1 {
		t.Fatalf("script = %q, want the only closing tag to be its own", got)
	}
}

func TestOnlyOneHalfOfTheStartRouteIsEnough(t *testing.T) {
	if StartScript("traffic", "") == "" {
		t.Fatal("a view with no context emitted nothing")
	}
	if StartScript("", "p-mk1") == "" {
		t.Fatal("a context with no view emitted nothing")
	}
}
