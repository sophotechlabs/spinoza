package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

// talkative is an apiserver that lists the pods of one workload and hands out a
// log body per pod that stays open, which is what a followed log looks like.
type talkative struct {
	mu     sync.Mutex
	pods   []string
	silent map[string]bool
}

func (c *talkative) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			c.serveLog(w, r)
			return
		}
		c.serveList(w)
	}
}

func (c *talkative) serveList(w http.ResponseWriter) {
	c.mu.Lock()
	names := append([]string{}, c.pods...)
	c.mu.Unlock()

	list := corev1.PodList{}
	for _, name := range names {
		list.Items = append(list.Items, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "prod"},
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (c *talkative) serveLog(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	name := c.known(parts[len(parts)-2])
	w.Header().Set("Content-Type", "text/plain")
	if name != "" && !c.quiet(name) {
		_, _ = fmt.Fprintf(w, "%s is up\n", name)
	}
	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	<-r.Context().Done()
}

func (c *talkative) scaleTo(names ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pods = names
}

// stayQuiet gives a pod a log that is open but empty, which is a pod that has
// nothing to say yet rather than one that is not being read.
func (c *talkative) stayQuiet(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.silent == nil {
		c.silent = map[string]bool{}
	}
	c.silent[name] = true
}

func (c *talkative) quiet(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.silent[name]
}

// known answers with the pod of that name this fake actually has, so what it
// writes back comes from its own list rather than from the request.
func (c *talkative) known(name string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, pod := range c.pods {
		if pod == name {
			return pod
		}
	}
	return ""
}

func selectingDeployment() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      "web",
			"namespace": "prod",
		},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{"app": "web"},
			},
		},
	}}
}

func workloadServer(t *testing.T, cs kubernetes.Interface) *httptest.Server {
	t.Helper()
	listKinds := map[schema.GroupVersionResource]string{
		{Version: "v1", Resource: "pods"}:                       "PodList",
		{Version: "v1", Resource: "events"}:                     "EventList",
		{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		listKinds,
		selectingDeployment(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("", "v1", "pods"): podDesc(),
	}
	mgr := resources.NewManager(ctx, resources.Deps{Dynamic: dyn, Clientset: cs, Descriptors: descs})
	ts := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func talkativeClient(t *testing.T, c *talkative) kubernetes.Interface {
	t.Helper()
	apiserver := httptest.NewServer(c.handler())
	t.Cleanup(apiserver.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: apiserver.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	return cs
}

func subscribeToWorkload(ctx context.Context, t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	sendMsg(ctx, t, conn, api.ClientMsg{
		Type:      "logs-subscribe",
		SubID:     "logs",
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "prod",
		Name:      "web",
		Follow:    true,
	})
	return conn
}

func TestWSTailsEveryPodOfAWorkload(t *testing.T) {
	cluster := &talkative{}
	cluster.scaleTo("web-0", "web-1")
	ts := workloadServer(t, talkativeClient(t, cluster))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	conn := subscribeToWorkload(ctx, t, ts)

	opened := readMsg(ctx, t, conn)
	if opened.Type != "log-open" {
		t.Fatalf("type = %q, want log-open first (message %q)", opened.Type, opened.Message)
	}
	if opened.Attached != 2 || opened.Matched != 2 {
		t.Fatalf("opened = %+v, want both pods of the deployment attached", opened)
	}

	said := map[string]string{}
	for len(said) < 2 {
		msg := readMsg(ctx, t, conn)
		if msg.Type != "log" {
			continue
		}
		if msg.Source == "" {
			t.Fatalf("a merged stream delivered a batch with no pod on it: %+v", msg)
		}
		said[msg.Source] = strings.Join(msg.Lines, "")
	}
	if said["web-0"] != "web-0 is up" || said["web-1"] != "web-1 is up" {
		t.Fatalf("lines = %v, want each pod's own output tagged with its name", said)
	}
}

func TestWSSaysWhenAPodJoinsAStreamThatHasGoneQuiet(t *testing.T) {
	restore := hurryPodCount(t)
	defer restore()
	cluster := &talkative{}
	cluster.scaleTo("web-0")
	cluster.stayQuiet("web-1")
	ts := workloadServer(t, talkativeClient(t, cluster))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn := subscribeToWorkload(ctx, t, ts)
	opened := readMsg(ctx, t, conn)
	if opened.Attached != 1 {
		t.Fatalf("opened = %+v, want the one pod there was", opened)
	}

	// web-1 writes nothing, so only a stream that looks at the pods on its own
	// will ever say that there are two of them.
	cluster.scaleTo("web-0", "web-1")

	for {
		msg := readMsg(ctx, t, conn)
		if msg.Type != "log-open" {
			continue
		}
		if msg.Attached != 2 || msg.Matched != 2 {
			continue
		}
		return
	}
}

func hurryPodCount(t *testing.T) func() {
	t.Helper()
	old := podCountInterval
	podCountInterval = 50 * time.Millisecond
	return func() {
		podCountInterval = old
	}
}
