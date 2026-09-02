package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStoppingOneBrowserWaiterLeavesTheOthersWaiting(t *testing.T) {
	state := views{}
	first := state.awaitBrowser()
	second := state.awaitBrowser()

	state.stopWaiting(second)

	if len(state.waiting) != 1 || state.waiting[0] != first {
		t.Fatalf("waiters = %v, want only the first waiter retained", state.waiting)
	}
	state.stopWaiting(second)
	if len(state.waiting) != 1 || state.waiting[0] != first {
		t.Fatalf("waiters = %v after repeated cleanup, want the first waiter retained", state.waiting)
	}
}

func TestCanceledBrowserSwitchRemovesItsWaiterAndKeepsTheWindowVisible(t *testing.T) {
	window := &stubWindow{}
	srv := New(&stubBackendCluster{}, testAssets(), "")
	srv.UseWindow(window)
	srv.UseBrowser(func() error { return nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/view/browser", http.NoBody).WithContext(ctx)
	recorder := httptest.NewRecorder()

	srv.toBrowser(recorder, req)

	moved := decodeSwitch(t, recorder.Body.Bytes())
	if moved.Switched || !strings.Contains(moved.Reason, "browser") {
		t.Fatalf("switch = %+v, want the canceled switch reported", moved)
	}
	if len(srv.views.waiting) != 0 {
		t.Fatalf("waiters = %d, want the canceled request removed", len(srv.views.waiting))
	}
	_, hidden := window.counts()
	if hidden != 0 {
		t.Fatalf("window hidden %d times, want it left visible", hidden)
	}
}
