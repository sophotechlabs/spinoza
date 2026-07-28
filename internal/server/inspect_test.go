package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

var (
	podGVR      = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	eventsGVR   = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
	objectQuery = "?version=v1&resource=pods&namespace=flux-system&name=web"
)

func podDesc() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Version:    "v1",
		Resource:   "pods",
		Kind:       "Pod",
		Namespaced: true,
		Category:   "Workloads",
	}
}

func newPod() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "web",
			"namespace": "flux-system",
			"uid":       "pod-uid",
		},
		"spec": map[string]interface{}{
			"containers": []interface{}{map[string]interface{}{"name": "app"}},
		},
	}}
}

func newPodEvent() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Event",
		"metadata": map[string]interface{}{
			"name":      "web.1",
			"namespace": "flux-system",
		},
		"involvedObject": map[string]interface{}{"uid": "pod-uid"},
		"type":           "Warning",
		"reason":         "BackOff",
		"message":        "restarting",
		"count":          int64(3),
		"lastTimestamp":  "2026-07-27T09:30:00Z",
		"source":         map[string]interface{}{"component": "kubelet"},
	}}
}

func inspectServer(t *testing.T, objs ...runtime.Object) *httptest.Server {
	t.Helper()
	return inspectServerWith(t, k8sfake.NewClientset(), objs...)
}

func inspectServerWith(t *testing.T, cs kubernetes.Interface, objs ...runtime.Object) *httptest.Server {
	t.Helper()
	listKinds := map[schema.GroupVersionResource]string{
		podGVR:    "PodList",
		eventsGVR: "EventList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objs...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("", "v1", "pods"): podDesc(),
	}
	mgr := resources.NewManager(ctx, dyn, cs, nil, nil, nil, descs)
	ts := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func failingLogClient(t *testing.T) kubernetes.Interface {
	t.Helper()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(broken.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: broken.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	return cs
}

func doRequest(t *testing.T, method, url string, body io.Reader) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("read body: %v", readErr)
	}
	return resp, payload
}

func TestGetObjectReturnsDetail(t *testing.T) {
	ts := inspectServer(t, newPod())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/object"+objectQuery, nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	var detail api.ObjectDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Name != "web" {
		t.Fatalf("name = %q, want web", detail.Name)
	}
	if !strings.Contains(detail.YAML, "kind: Pod") {
		t.Fatalf("yaml = %q", detail.YAML)
	}
	if len(detail.Containers) != 1 || detail.Containers[0] != "app" {
		t.Fatalf("containers = %v", detail.Containers)
	}
}

func TestGetObjectRequiresParams(t *testing.T) {
	ts := inspectServer(t)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/object?version=v1", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "required") {
		t.Fatalf("body = %s", body)
	}
}

func TestGetObjectNotFound(t *testing.T) {
	ts := inspectServer(t)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/object"+objectQuery, nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, body)
	}
}

func TestPutObjectApplies(t *testing.T) {
	ts := inspectServer(t, newPod())
	doc := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n  namespace: flux-system\n  labels:\n    app: edited\n"

	resp, body := doRequest(t, http.MethodPut, ts.URL+"/api/object"+objectQuery, strings.NewReader(doc))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	var detail api.ObjectDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Labels["app"] != "edited" {
		t.Fatalf("labels = %v", detail.Labels)
	}
}

func TestPutObjectRejectsMismatch(t *testing.T) {
	ts := inspectServer(t, newPod())
	doc := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: other\n  namespace: flux-system\n"

	resp, body := doRequest(t, http.MethodPut, ts.URL+"/api/object"+objectQuery, strings.NewReader(doc))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "document name") {
		t.Fatalf("body = %s", body)
	}
}

func TestDeleteObject(t *testing.T) {
	ts := inspectServer(t, newPod())

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/object"+objectQuery, nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.StatusCode, body)
	}

	missing, _ := doRequest(t, http.MethodGet, ts.URL+"/api/object"+objectQuery, nil)
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("object still present, status = %d", missing.StatusCode)
	}
}

func TestDeleteObjectNotFound(t *testing.T) {
	ts := inspectServer(t)

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/object"+objectQuery, nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestObjectRejectsUnsupportedMethod(t *testing.T) {
	ts := inspectServer(t, newPod())

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/object"+objectQuery, strings.NewReader("{}"))

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestEventsEndpoint(t *testing.T) {
	ts := inspectServer(t, newPod(), newPodEvent())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/events?namespace=flux-system&uid=pod-uid", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	var events []api.Event
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Reason != "BackOff" {
		t.Fatalf("reason = %q", events[0].Reason)
	}
}

func TestWSStreamsPodLogs(t *testing.T) {
	ts := inspectServer(t, newPod())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.CloseNow() }()

	sendMsg(ctx, t, c, api.ClientMsg{
		Type:      "logs-subscribe",
		SubID:     "logs",
		Namespace: "flux-system",
		Name:      "web",
		Container: "app",
		TailLines: 100,
	})

	msg := readMsg(ctx, t, c)
	if msg.Type != "log" {
		t.Fatalf("type = %q, want log (message %q)", msg.Type, msg.Message)
	}
	if msg.SubID != "logs" {
		t.Fatalf("subId = %q, want logs", msg.SubID)
	}
	if len(msg.Lines) == 0 {
		t.Fatalf("no log lines delivered")
	}

	end := readMsg(ctx, t, c)
	if end.Type != "log-end" {
		t.Fatalf("type = %q, want log-end", end.Type)
	}
}

func TestWSLogsUnsubscribe(t *testing.T) {
	ts := inspectServer(t, newPod())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.CloseNow() }()

	sendMsg(ctx, t, c, api.ClientMsg{Type: "logs-subscribe", SubID: "logs", Namespace: "flux-system", Name: "web"})
	readMsg(ctx, t, c)
	sendMsg(ctx, t, c, api.ClientMsg{Type: "logs-unsubscribe", SubID: "logs"})

	sendMsg(ctx, t, c, api.ClientMsg{Type: "logs-subscribe", SubID: "second", Namespace: "flux-system", Name: "web"})
	for {
		msg := readMsg(ctx, t, c)
		if msg.SubID == "second" {
			return
		}
	}
}

func sendMsg(ctx context.Context, t *testing.T, c *websocket.Conn, msg api.ClientMsg) {
	t.Helper()
	if err := wsjson.Write(ctx, c, msg); err != nil {
		t.Fatalf("write message: %v", err)
	}
}

func TestWSLogsReportsOpenFailure(t *testing.T) {
	ts := inspectServerWith(t, failingLogClient(t), newPod())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.CloseNow() }()

	sendMsg(ctx, t, c, api.ClientMsg{Type: "logs-subscribe", SubID: "logs", Namespace: "flux-system", Name: "web"})

	msg := readMsg(ctx, t, c)
	if msg.Type != "error" {
		t.Fatalf("type = %q, want error", msg.Type)
	}
	if msg.Message == "" {
		t.Fatalf("error message is empty")
	}
}

func TestPutObjectRejectsOversizedBody(t *testing.T) {
	ts := inspectServer(t, newPod())
	body := strings.NewReader(strings.Repeat("a", (4<<20)+1))

	resp, _ := doRequest(t, http.MethodPut, ts.URL+"/api/object"+objectQuery, body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestStatusForMapsAPIErrors(t *testing.T) {
	gr := schema.GroupResource{Resource: "pods"}
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", apierrors.NewNotFound(gr, "web"), http.StatusNotFound},
		{"conflict", apierrors.NewConflict(gr, "web", errors.New("changed")), http.StatusConflict},
		{"forbidden", apierrors.NewForbidden(gr, "web", errors.New("denied")), http.StatusForbidden},
		{"unauthorized", apierrors.NewUnauthorized("no token"), http.StatusForbidden},
		{"invalid", apierrors.NewInvalid(schema.GroupKind{Kind: "Pod"}, "web", nil), http.StatusUnprocessableEntity},
		{"bad request", apierrors.NewBadRequest("nope"), http.StatusUnprocessableEntity},
		{"plain", errors.New("boom"), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := statusFor(tc.err)
			if got != tc.want {
				t.Fatalf("statusFor(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

func TestBatchLinesDrainsWhatIsBuffered(t *testing.T) {
	lines := make(chan string, 4)
	lines <- "second"
	lines <- "third"

	batch := batchLines(lines, "first")

	if strings.Join(batch, ",") != "first,second,third" {
		t.Fatalf("batch = %v", batch)
	}
}

func TestBatchLinesStopsAtClosedChannel(t *testing.T) {
	lines := make(chan string, 2)
	lines <- "second"
	close(lines)

	batch := batchLines(lines, "first")

	if strings.Join(batch, ",") != "first,second" {
		t.Fatalf("batch = %v", batch)
	}
}

func TestBatchLinesStopsAtTheBatchCap(t *testing.T) {
	lines := make(chan string, maxLogBatch*2)
	for i := 0; i < maxLogBatch*2; i++ {
		lines <- "line"
	}

	batch := batchLines(lines, "first")

	if len(batch) != maxLogBatch {
		t.Fatalf("batch = %d lines, want %d", len(batch), maxLogBatch)
	}
}
