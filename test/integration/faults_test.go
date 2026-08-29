//go:build integration

package integration

import (
	"context"
	"os/exec"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/kube"
)

const faultsNamespace = "spinoza-faults"

const (
	faultPoll    = 5 * time.Second
	faultTimeout = 3 * time.Minute
)

func faultsNamespaceExists(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	space := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: faultsNamespace}}
	_, err := loaded.Clientset.CoreV1().Namespaces().Create(context.Background(), space, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.CoreV1().Namespaces().Delete(
			context.Background(), faultsNamespace, metav1.DeleteOptions{},
		)
	})
}

func unbindableClaim(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	nothing := "spinoza-no-such-class"
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "never-binds", Namespace: faultsNamespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &nothing,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	_, err := loaded.Clientset.CoreV1().PersistentVolumeClaims(faultsNamespace).Create(
		context.Background(), claim, metav1.CreateOptions{},
	)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create claim: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.CoreV1().PersistentVolumeClaims(faultsNamespace).Delete(
			context.Background(), "never-binds", metav1.DeleteOptions{},
		)
	})
}

func serviceSelectingNothing(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "answers-nothing", Namespace: faultsNamespace},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "nothing-has-this-label"},
			Ports:    []corev1.ServicePort{{Port: 80}},
		},
	}
	_, err := loaded.Clientset.CoreV1().Services(faultsNamespace).Create(
		context.Background(), service, metav1.CreateOptions{},
	)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create service: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.CoreV1().Services(faultsNamespace).Delete(
			context.Background(), "answers-nothing", metav1.DeleteOptions{},
		)
	})
}

// A finalizer nothing removes is what makes a delete hang, which is the state
// the detector is for. Cleanup strips it so the namespace can go.
func podHeldByAFinalizer(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	pods := loaded.Clientset.CoreV1().Pods(faultsNamespace)
	grace := int64(1)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "will-not-go",
			Namespace:  faultsNamespace,
			Finalizers: []string{"spinoza.tech/held-for-the-audit"},
		},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &grace,
			Containers: []corev1.Container{{
				Name:    "app",
				Image:   "busybox:1.36",
				Command: []string{"sh", "-c", "sleep 3600"},
			}},
		},
	}
	_, err := pods.Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create pod: %v", err)
	}
	t.Cleanup(func() {
		clear := []byte(`{"metadata":{"finalizers":null}}`)
		_, _ = pods.Patch(context.Background(), "will-not-go", types.MergePatchType, clear, metav1.PatchOptions{})
		_ = pods.Delete(context.Background(), "will-not-go", metav1.DeleteOptions{})
	})
	_ = pods.Delete(context.Background(), "will-not-go", metav1.DeleteOptions{})
}

func cordonedNode(t *testing.T, loaded *kube.Bundle) string {
	t.Helper()
	name := faultNode(t, loaded, 1)
	patchNode(t, loaded, name, `{"spec":{"unschedulable":true}}`)
	t.Cleanup(func() {
		patchNode(t, loaded, name, `{"spec":{"unschedulable":null}}`)
	})
	return name
}

func faultNode(t *testing.T, loaded *kube.Bundle, at int) string {
	t.Helper()
	nodes, err := loaded.Clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil || len(nodes.Items) <= at {
		t.Fatalf("this cluster has %d nodes, and the test wants at least %d (%v)", len(nodes.Items), at+1, err)
	}
	return nodes.Items[at].Name
}

func patchNode(t *testing.T, loaded *kube.Bundle, name, body string) {
	t.Helper()
	_, err := loaded.Clientset.CoreV1().Nodes().Patch(
		context.Background(), name, types.StrategicMergePatchType, []byte(body), metav1.PatchOptions{},
	)
	if err != nil {
		t.Fatalf("patch node %s: %v", name, err)
	}
}

// kind names the container after the node, so stopping the kubelet is one
// docker exec. The node reports NotReady once the controller manager's grace
// period runs out, which is why the wait below is generous.
func nodeWithNoKubelet(t *testing.T, loaded *kube.Bundle) string {
	t.Helper()
	name := faultNode(t, loaded, 2)
	if err := kubeletOn(name, "stop"); err != nil {
		t.Skipf("could not stop the kubelet on %s, so NotReady was not exercised: %v", name, err)
	}
	t.Cleanup(func() {
		_ = kubeletOn(name, "start")
	})
	return name
}

func kubeletOn(node, action string) error {
	bounded, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(bounded, "docker", "exec", node, "systemctl", action, "kubelet").CombinedOutput()
	if err != nil {
		return &kubeletError{action: action, node: node, output: string(out), cause: err}
	}
	return nil
}

type kubeletError struct {
	action string
	node   string
	output string
	cause  error
}

func (e *kubeletError) Error() string {
	return "systemctl " + e.action + " kubelet on " + e.node + ": " + e.cause.Error() + ": " + e.output
}

func firedOn(queue api.IssueQueue, title, name string) bool {
	for _, row := range queue.Rows {
		if row.Title == title && row.Object.Name == name {
			return true
		}
		for _, child := range row.Children {
			if child.Object.Name == name {
				return true
			}
		}
	}
	return false
}

func waitForRow(t *testing.T, mgr issueSource, title, name string) {
	t.Helper()
	deadline := time.Now().Add(faultTimeout)
	for time.Now().Before(deadline) {
		if firedOn(mgr.Issues(context.Background()), title, name) {
			return
		}
		time.Sleep(faultPoll)
	}
	t.Errorf("%s never appeared for %s within %s", title, name, faultTimeout)
}

type issueSource interface {
	Issues(ctx context.Context) api.IssueQueue
}

// the runtime faults, forced on a real cluster

func TestTheClusterFaultDetectorsFireOnARealCluster(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	faultsNamespaceExists(t, loaded)
	unbindableClaim(t, loaded)
	serviceSelectingNothing(t, loaded)
	podHeldByAFinalizer(t, loaded)
	cordoned := cordonedNode(t, loaded)

	waitForRow(t, mgr, "ClaimPending", "never-binds")
	waitForRow(t, mgr, "NoEndpoints", "answers-nothing")
	waitForRow(t, mgr, "Cordoned", cordoned)
}

func TestANodeWhoseKubeletStoppedIsReported(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	stopped := nodeWithNoKubelet(t, loaded)

	deadline := time.Now().Add(faultTimeout)
	for time.Now().Before(deadline) {
		queue := mgr.Issues(context.Background())
		for _, row := range queue.Rows {
			if row.Object.Name != stopped {
				continue
			}
			if row.Title == "Cordoned" {
				continue
			}
			return
		}
		time.Sleep(faultPoll)
	}
	t.Errorf("%s never showed a fault after its kubelet stopped", stopped)
}
