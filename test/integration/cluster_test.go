//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const namespace = "spinoza-smoke"

func bundle(t *testing.T) *kube.Bundle {
	t.Helper()
	loaded, err := kube.LoadContext(os.Getenv("SPINOZA_TEST_CONTEXT"), kube.Options{})
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	return loaded
}

func manager(t *testing.T, loaded *kube.Bundle) *resources.Manager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cats, descs, err := discovery.List(loaded.Discovery)
	if err != nil {
		t.Logf("discovery came back partial: %v", err)
	}
	if len(descs) == 0 {
		t.Fatal("the cluster listed no resource types")
	}
	return resources.NewManager(ctx, resources.Deps{
		Dynamic:     loaded.Dynamic,
		Clientset:   loaded.Clientset,
		Categories:  cats,
		Descriptors: descs,
	})
}

func ensureNamespace(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	ctx := context.Background()
	_, err := loaded.Clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	})
}

func runningPod(t *testing.T, loaded *kube.Bundle, name string) *corev1.Pod {
	t.Helper()
	ctx := context.Background()
	created, err := loaded.Clientset.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"app": name}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    "web",
				Image:   "busybox:1.36",
				Command: []string{"sh", "-c", "sleep 3600"},
				Ports:   []corev1.ContainerPort{{ContainerPort: 80}},
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create pod: %v", err)
	}
	if err == nil {
		created = waitRunning(t, loaded, created.Name)
	}
	return created
}

func waitRunning(t *testing.T, loaded *kube.Bundle, name string) *corev1.Pod {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		pod, err := loaded.Clientset.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err == nil && pod.Status.Phase == corev1.PodRunning {
			return pod
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("pod %s never reached Running", name)
	return nil
}

func TestSubscribeSeesARealInformerFill(t *testing.T) {
	loaded := bundle(t)
	ensureNamespace(t, loaded)
	mgr := manager(t, loaded)
	runningPod(t, loaded, "smoke-subscribe")

	sub, err := mgr.Subscribe(context.Background(), "", "v1", "pods", namespace)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)

	rows, snapErr := sub.Snapshot()
	if snapErr != nil {
		t.Fatalf("snapshot: %v", snapErr)
	}
	if len(rows) == 0 {
		t.Fatal("a real informer synced with no rows")
	}
	if len(sub.Columns) == 0 {
		t.Fatal("the subscription carried no columns")
	}
}

func TestApplyReportsARealConflict(t *testing.T) {
	loaded := bundle(t)
	ensureNamespace(t, loaded)
	runningPod(t, loaded, "smoke-conflict")
	ref := api.ObjectRef{Version: "v1", Resource: "pods", Namespace: namespace, Name: "smoke-conflict"}

	detail, err := inspect.Get(context.Background(), loaded.Dynamic, ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	stale := detail.YAML
	if _, err := loaded.Clientset.CoreV1().Pods(namespace).Patch(context.Background(), "smoke-conflict",
		"application/merge-patch+json", []byte(`{"metadata":{"labels":{"touched":"yes"}}}`), metav1.PatchOptions{}); err != nil {
		t.Fatalf("patch: %v", err)
	}

	_, applyErr := inspect.Apply(context.Background(), loaded.Dynamic, ref, "Pod", []byte(stale))

	if applyErr == nil {
		t.Fatal("applying a stale resourceVersion reported success; a fake clientset never produces this")
	}
	if !apierrors.IsConflict(applyErr) {
		t.Fatalf("err = %v, want a 409 conflict", applyErr)
	}
}

func TestCountsReachEveryDiscoveredType(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)

	counts := mgr.Counts(context.Background())

	if len(counts.Counts) == 0 {
		t.Fatal("counting produced nothing against a real apiserver")
	}
	pods, ok := counts.Counts["/v1/pods"]
	if !ok {
		t.Fatalf("pods were not counted: %v", counts.Errors)
	}
	if pods < 0 {
		t.Fatalf("pods = %d (%s)", pods, counts.Errors["/v1/pods"])
	}
}

func TestPortForwardReachesARealPod(t *testing.T) {
	loaded := bundle(t)
	ensureNamespace(t, loaded)
	runningPod(t, loaded, "smoke-forward")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := portforward.NewRegistry(ctx,
		portforward.NewRunner(loaded.Clientset, loaded.Config),
		portforward.NewResolver(loaded.Clientset),
		portforward.NewProber(loaded.Clientset),
	)
	t.Cleanup(registry.StopAll)

	forward, err := registry.Start(context.Background(),
		portforward.Target{Kind: portforward.KindPod, Namespace: namespace, Name: "smoke-forward"}, 80)
	if err != nil {
		t.Fatalf("start forward: %v", err)
	}

	if forward.LocalPort == 0 {
		t.Fatal("the forward reported no local port")
	}
	if forward.State != portforward.StateRunning {
		t.Fatalf("state = %q (%s)", forward.State, forward.Error)
	}
}

type collected struct {
	mu    sync.Mutex
	bytes []byte
}

func (c *collected) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bytes = append(c.bytes, p...)
	return len(p), nil
}

func (c *collected) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.bytes)
}

func TestExecRunsAShellInARealPod(t *testing.T) {
	loaded := bundle(t)
	ensureNamespace(t, loaded)
	runningPod(t, loaded, "smoke-exec")
	service := exec.NewService(
		exec.NewStreamer(loaded.Clientset, loaded.Config),
		exec.NewImages(loaded.Clientset),
	)
	output := &collected{}

	session, err := service.Start(context.Background(),
		exec.Request{Namespace: namespace, Pod: "smoke-exec", Container: "web"}, output)
	if err != nil {
		t.Fatalf("start exec: %v", err)
	}
	t.Cleanup(session.Close)

	if _, err := session.Write([]byte("echo spinoza-was-here\n")); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), "spinoza-was-here") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("the shell never echoed back: %q", output.String())
}

func TestListWarmsAPinnedInformerAgainstARealCluster(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	desc := api.ResourceDescriptor{Version: "v1", Resource: "namespaces", Kind: "Namespace"}

	items, err := mgr.List(context.Background(), desc)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(items) == 0 {
		t.Fatal("a real cluster listed no namespaces")
	}
	if _, ok := items[0].Object["metadata"]; !ok {
		t.Fatalf("item carried no metadata: %v", items[0])
	}
	mgr.Unpin([]api.ResourceDescriptor{desc})
}
