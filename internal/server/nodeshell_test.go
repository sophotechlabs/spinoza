package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/nodeshell"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func shellCluster(t *testing.T) *k8sfake.Clientset {
	t.Helper()
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.SelfSubjectAccessReview{
			Status: authv1.SubjectAccessReviewStatus{Allowed: true},
		}, nil
	})
	cs.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		pod, ok := create.GetObject().(*corev1.Pod)
		if !ok {
			return false, nil, nil
		}
		pod.Name = "spinoza-node-shell-abc"
		pod.Status.Phase = corev1.PodRunning
		return false, pod, nil
	})
	return cs
}

func nodeShellServer(t *testing.T, cs *k8sfake.Clientset, images *fakeImages, enabled bool) *httptest.Server {
	t.Helper()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{podGVR: "PodList"},
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	shell := newFakeShell()
	shell.greet = "/ # "
	mgr := resources.NewManager(ctx, resources.Deps{
		Dynamic:   dyn,
		Clientset: cs,
		Shells:    exec.NewService(shell, images),
		NodeShells: nodeshell.NewService(
			cs,
			"busybox:1.37",
			nodeshell.DefaultNamespace,
			func() bool { return enabled },
			access.New(cs),
		),
	})
	ts := httptest.NewServer(authed(New(fixed(mgr), testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func shellPods(t *testing.T, cs *k8sfake.Clientset) int {
	t.Helper()
	pods, err := cs.CoreV1().Pods(nodeshell.DefaultNamespace).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=spinoza"},
	)
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	return len(pods.Items)
}

func dialNodeShell(t *testing.T, ts *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/nodeshell" + query
	conn, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func TestNodeShellSupportNeedsANode(t *testing.T) {
	ts := nodeShellServer(t, shellCluster(t), &fakeImages{digest: "sha256:shelled"}, true)

	resp := getJSON(t, ts.URL+"/api/nodeshell/support", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestNodeShellSupportAnswersForTheNodeItWasAskedAbout(t *testing.T) {
	ts := nodeShellServer(t, shellCluster(t), &fakeImages{digest: "sha256:shelled"}, true)

	var support api.NodeShellSupport
	resp := getJSON(t, ts.URL+"/api/nodeshell/support?node=p-mk1", &support)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if support.Node != "p-mk1" || !support.Enabled || !support.Allowed {
		t.Fatalf("support = %+v, want it on and allowed", support)
	}
	if support.Image != "busybox:1.37" || support.Namespace != nodeshell.DefaultNamespace {
		t.Fatalf("support = %+v, want the image and namespace it would use", support)
	}
}

func TestNodeShellSupportSaysWhenItIsOff(t *testing.T) {
	ts := nodeShellServer(t, shellCluster(t), &fakeImages{digest: "sha256:shelled"}, false)

	var support api.NodeShellSupport
	getJSON(t, ts.URL+"/api/nodeshell/support?node=p-mk1", &support)

	if support.Enabled {
		t.Fatalf("support = %+v, want it off", support)
	}
	if !strings.Contains(support.Reason, "--node-shell") {
		t.Fatalf("reason = %q, want it to say how to turn them on", support.Reason)
	}
}

func TestANodeShellNeedsANode(t *testing.T) {
	ts := nodeShellServer(t, shellCluster(t), &fakeImages{digest: "sha256:shelled"}, true)

	resp := getJSON(t, ts.URL+"/api/nodeshell", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestANodeShellRunsAPodAndTakesItAwayWhenTheSocketCloses(t *testing.T) {
	cs := shellCluster(t)
	ts := nodeShellServer(t, cs, &fakeImages{digest: "sha256:shelled"}, true)

	conn := dialNodeShell(t, ts, "?node=p-mk1")
	channel, payload := readFrame(t, conn)

	if channel != api.ExecChannelStdout || string(payload) != "/ # " {
		t.Fatalf("first frame = %d %q, want the shell greeting", channel, payload)
	}
	if shellPods(t, cs) != 1 {
		t.Fatalf("pods = %d, want the one the shell runs on", shellPods(t, cs))
	}

	_ = conn.CloseNow()

	waitForServer(t, func() bool {
		return shellPods(t, cs) == 0
	}, "the privileged pod outlived the socket that opened it")
}

func TestANodeShellThatCannotStartLeavesNoPodBehind(t *testing.T) {
	cs := shellCluster(t)
	ts := nodeShellServer(t, cs, &fakeImages{err: errors.New("the container has no image id")}, true)

	conn := dialNodeShell(t, ts, "?node=p-mk1")
	channel, payload := readFrame(t, conn)

	if channel != api.ExecChannelError {
		t.Fatalf("channel = %d, want the failure reported", channel)
	}
	if !strings.Contains(string(payload), "no image id") {
		t.Fatalf("payload = %q, want what went wrong", payload)
	}
	waitForServer(t, func() bool {
		return shellPods(t, cs) == 0
	}, "a shell that never opened still left its privileged pod running")
}

func TestANodeShellThatIsOffCreatesNothing(t *testing.T) {
	cs := shellCluster(t)
	ts := nodeShellServer(t, cs, &fakeImages{digest: "sha256:shelled"}, false)

	conn := dialNodeShell(t, ts, "?node=p-mk1")
	channel, payload := readFrame(t, conn)

	if channel != api.ExecChannelError {
		t.Fatalf("channel = %d, want the refusal", channel)
	}
	if !strings.Contains(string(payload), "node shells are off") {
		t.Fatalf("payload = %q", payload)
	}
	if shellPods(t, cs) != 0 {
		t.Fatalf("pods = %d, want none while node shells are off", shellPods(t, cs))
	}
}

func TestANodeShellEntersTheNodesOwnNamespaces(t *testing.T) {
	cs := shellCluster(t)
	created := make(chan *corev1.Pod, 1)
	cs.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if ok {
			pod, isPod := create.GetObject().(*corev1.Pod)
			if isPod {
				select {
				case created <- pod:
				default:
				}
			}
		}
		return false, nil, nil
	})
	ts := nodeShellServer(t, cs, &fakeImages{digest: "sha256:shelled"}, true)

	conn := dialNodeShell(t, ts, "?node=p-mk1")
	readFrame(t, conn)

	pod := <-created
	if pod.Spec.NodeName != "p-mk1" {
		t.Fatalf("node = %q, want the one that was asked for", pod.Spec.NodeName)
	}
	if !pod.Spec.HostPID || !pod.Spec.HostNetwork || !pod.Spec.HostIPC {
		t.Fatalf("pod = %+v, want the host namespaces", pod.Spec)
	}
	if pod.Spec.ActiveDeadlineSeconds == nil {
		t.Fatal("a privileged pod was given no deadline to die by")
	}
}

func TestTheNodeShellEndpointsRefuseAnythingButGet(t *testing.T) {
	ts := nodeShellServer(t, shellCluster(t), &fakeImages{digest: "sha256:shelled"}, true)

	for _, path := range []string{"/api/nodeshell", "/api/nodeshell/support"} {
		resp, _ := doRequest(t, http.MethodPost, ts.URL+path+"?node=p-mk1", http.NoBody)
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s = %d, want 405", path, resp.StatusCode)
		}
	}
}

func TestTheNodeShellSupportAnswerIsJSON(t *testing.T) {
	ts := nodeShellServer(t, shellCluster(t), &fakeImages{digest: "sha256:shelled"}, true)

	resp := getJSON(t, ts.URL+"/api/nodeshell/support?node=p-mk1", nil)

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", resp.Header.Get("Content-Type"))
	}
	var body map[string]json.RawMessage
	decodeErr := json.NewDecoder(resp.Body).Decode(&body)
	if decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if _, ok := body["enabled"]; !ok {
		t.Fatalf("body = %v, want the enabled flag", body)
	}
}
