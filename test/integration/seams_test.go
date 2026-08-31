//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/cluster"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const limitedUser = "spinoza-limited"

func limitedClients(t *testing.T, loaded *kube.Bundle) (dynamic.Interface, kubernetes.Interface) {
	t.Helper()
	ctx := context.Background()
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "spinoza-limited-pods"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		}},
	}
	_, err := loaded.Clientset.RbacV1().ClusterRoles().Create(ctx, role, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create role: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.RbacV1().ClusterRoles().Delete(context.Background(), role.Name, metav1.DeleteOptions{})
	})
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "spinoza-limited-pods"},
		Subjects: []rbacv1.Subject{{
			Kind:     rbacv1.UserKind,
			Name:     limitedUser,
			APIGroup: rbacv1.GroupName,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     role.Name,
		},
	}
	_, err = loaded.Clientset.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create binding: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.RbacV1().ClusterRoleBindings().Delete(context.Background(), binding.Name, metav1.DeleteOptions{})
	})

	cfg := rest.CopyConfig(loaded.Config)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: limitedUser}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("dynamic: %v", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	awaitPodsAllowed(t, cs)
	return dyn, cs
}

func awaitPodsAllowed(t *testing.T, cs kubernetes.Interface) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		_, last = cs.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{Limit: 1})
		if last == nil {
			return
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("the limited user never got its pod grant: %v", last)
}

func limitedManager(t *testing.T, loaded *kube.Bundle, dyn dynamic.Interface, cs kubernetes.Interface) *resources.Manager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cats, descs, err := discovery.List(loaded.Discovery)
	if err != nil {
		t.Logf("discovery came back partial: %v", err)
	}
	mgr := resources.NewManager(ctx, resources.Deps{
		Limits:      resources.Limits{SyncTimeout: 30 * time.Second},
		Dynamic:     dyn,
		Clientset:   cs,
		Categories:  cats,
		Descriptors: descs,
	})
	mgr.UseDiscovery(loaded.Discovery, err)
	return mgr
}

func TestAForbiddenListFailsFastForALimitedUser(t *testing.T) {
	loaded := bundle(t)
	dyn, cs := limitedClients(t, loaded)
	mgr := limitedManager(t, loaded, dyn, cs)

	allowed, err := mgr.Subscribe(context.Background(), "", "v1", "pods", namespace, 0, nil)
	if err != nil {
		t.Fatalf("subscribe pods as the limited user: %v", err)
	}
	allowed.Close()

	start := time.Now()
	_, err = mgr.Subscribe(context.Background(), "", "v1", "secrets", "", 0, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("the limited user listed secrets")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("err = %v, want the apiserver denial", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("subscribe took %s, want the denial before the sync window runs out", elapsed)
	}
}

func TestAServiceProxyDenialNamesThePermission(t *testing.T) {
	loaded := bundle(t)
	_, cs := limitedClients(t, loaded)

	client := prom.NewClient(cs, prom.Target{Namespace: namespace, Service: "missing", Port: "9090"})
	_, err := client.Target(context.Background())

	if err == nil {
		t.Fatal("the limited user proxied a service")
	}
	if !errors.Is(err, prom.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if !strings.Contains(err.Error(), "services/proxy") {
		t.Fatalf("err = %v, want the missing permission named", err)
	}
}

func TestNewWiresARealCluster(t *testing.T) {
	bundle(t)
	raw, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	name := os.Getenv("SPINOZA_TEST_CONTEXT")
	if _, ok := raw.Contexts[name]; !ok {
		t.Fatalf("context %q is not in the default kubeconfig", name)
	}
	raw.CurrentContext = name
	path := filepath.Join(t.TempDir(), "kubeconfig")
	writeErr := clientcmd.WriteToFile(*raw, path)
	if writeErr != nil {
		t.Fatalf("write kubeconfig: %v", writeErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	built, err := cluster.New(ctx, cluster.Options{Kubeconfig: path, SyncTimeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if built.Manager("") == nil {
		t.Fatalf("no manager came up: %s", built.Contexts().Error)
	}
	if built.Current().Name != name {
		t.Fatalf("current = %q, want %q", built.Current().Name, name)
	}
	if built.Contexts().Error != "" {
		t.Fatalf("error = %q, want none", built.Contexts().Error)
	}
	if err := built.Use(api.ContextRef{Name: name}); err != nil {
		t.Fatalf("use: %v", err)
	}
}

func startFakeProm(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	ctx := context.Background()
	serve := "mkdir -p /www/api/v1/status" +
		" && echo '{\"status\":\"success\"}' > /www/api/v1/status/buildinfo" +
		" && httpd -f -p 9090 -h /www"
	_, err := loaded.Clientset.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "fake-prom", Labels: map[string]string{"app": "fake-prom"}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    "web",
				Image:   "busybox:1.36",
				Command: []string{"sh", "-c", serve},
				Ports:   []corev1.ContainerPort{{ContainerPort: 9090}},
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create pod: %v", err)
	}
	_, err = loaded.Clientset.CoreV1().Services(namespace).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "fake-prom"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "fake-prom"},
			Ports:    []corev1.ServicePort{{Port: 9090, TargetPort: intstr.FromInt32(9090)}},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create service: %v", err)
	}
	waitRunning(t, loaded, "fake-prom")
}

func TestPrometheusProbeThroughARealProxy(t *testing.T) {
	loaded := bundle(t)
	startFakeProm(t, loaded)

	client := prom.NewClient(loaded.Clientset, prom.Target{Namespace: namespace, Service: "fake-prom", Port: "9090"})
	deadline := time.Now().Add(2 * time.Minute)
	var target prom.Target
	var err error
	for {
		target, err = client.Target(context.Background())
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the probe never reached the fake prometheus: %v", err)
		}
		time.Sleep(pollInterval)
	}
	if target.Scheme != "http" {
		t.Fatalf("scheme = %q, want the http fallback against a plain server", target.Scheme)
	}
}
