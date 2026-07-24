package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/broker"
)

type fakeBroker struct {
	rows   []api.PodRow
	rv     string
	events chan broker.Event
}

func (b *fakeBroker) Snapshot() ([]api.PodRow, string) {
	return b.rows, b.rv
}

func (b *fakeBroker) Subscribe() (<-chan broker.Event, func()) {
	return b.events, func() {}
}

func newFakeBroker() *fakeBroker {
	return &fakeBroker{
		rows:   []api.PodRow{{UID: "uid-1", Name: "pod-a", Namespace: "ns-a"}},
		rv:     "7",
		events: make(chan broker.Event, 4),
	}
}

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spinoza-index</html>")},
	}
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + "/ws"
}

func TestHealthzReturnsOK(t *testing.T) {
	srv := New(newFakeBroker(), testAssets())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", string(body))
	}
}

func TestRootServesSPAIndex(t *testing.T) {
	srv := New(newFakeBroker(), testAssets())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "spinoza-index") {
		t.Fatalf("body = %q, want SPA index", string(body))
	}
}

func TestEventToMsgDeleted(t *testing.T) {
	msg := eventToMsg(broker.Event{Kind: "deleted", UID: "uid-1"})
	if msg.Type != "deleted" {
		t.Fatalf("Type = %q, want deleted", msg.Type)
	}
	if msg.Resource != "pods" {
		t.Fatalf("Resource = %q, want pods", msg.Resource)
	}
	if msg.UID != "uid-1" {
		t.Fatalf("UID = %q, want uid-1", msg.UID)
	}
	if msg.Item != nil {
		t.Fatalf("Item = %v, want nil", msg.Item)
	}
}

func TestEventToMsgAdded(t *testing.T) {
	row := api.PodRow{UID: "uid-2", Name: "pod-b"}
	msg := eventToMsg(broker.Event{Kind: "added", Row: row})
	if msg.Type != "added" {
		t.Fatalf("Type = %q, want added", msg.Type)
	}
	if msg.Item == nil {
		t.Fatal("Item = nil, want row")
	}
	if msg.Item.UID != "uid-2" {
		t.Fatalf("Item.UID = %q, want uid-2", msg.Item.UID)
	}
}

func TestHandleWSDeliversSnapshotAndDelta(t *testing.T) {
	b := newFakeBroker()
	srv := New(b, testAssets())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.CloseNow() }()

	var snap api.ServerMsg
	if err := wsjson.Read(ctx, c, &snap); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if snap.Type != "snapshot" {
		t.Fatalf("snapshot Type = %q, want snapshot", snap.Type)
	}
	if snap.RV != "7" {
		t.Fatalf("snapshot RV = %q, want 7", snap.RV)
	}
	if len(snap.Items) != 1 {
		t.Fatalf("snapshot Items = %d, want 1", len(snap.Items))
	}

	b.events <- broker.Event{Kind: "added", Row: api.PodRow{UID: "uid-3", Name: "pod-c"}}

	var delta api.ServerMsg
	if err := wsjson.Read(ctx, c, &delta); err != nil {
		t.Fatalf("read delta: %v", err)
	}
	if delta.Type != "added" {
		t.Fatalf("delta Type = %q, want added", delta.Type)
	}
	if delta.Item == nil || delta.Item.UID != "uid-3" {
		t.Fatalf("delta Item = %v, want uid-3", delta.Item)
	}
}

func TestHandleWSExitsWhenEventChannelCloses(t *testing.T) {
	b := newFakeBroker()
	srv := New(b, testAssets())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.CloseNow() }()

	var snap api.ServerMsg
	if err := wsjson.Read(ctx, c, &snap); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	close(b.events)

	_, _, readErr := c.Read(ctx)
	if readErr == nil {
		t.Fatal("expected connection to close after event channel closed")
	}
}

func TestHandleWSExitsOnContextCancel(t *testing.T) {
	b := newFakeBroker()
	srv := New(b, testAssets())

	ts := httptest.NewUnstartedServer(srv.Handler())
	baseCtx, cancelBase := context.WithCancel(context.Background())
	ts.Config.BaseContext = func(_ net.Listener) context.Context {
		return baseCtx
	}
	ts.Start()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.CloseNow() }()

	var snap api.ServerMsg
	if err := wsjson.Read(ctx, c, &snap); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	cancelBase()

	_, _, readErr := c.Read(ctx)
	if readErr == nil {
		t.Fatal("expected connection to close after context cancel")
	}
}

func TestHandleWSRejectsNonWebsocketRequest(t *testing.T) {
	srv := New(newFakeBroker(), testAssets())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ws")
	if err != nil {
		t.Fatalf("GET /ws: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want a non-upgrade rejection", resp.StatusCode)
	}
}
