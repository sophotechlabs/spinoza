//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func TestANamespaceRoleAloneCanBrowseItsNamespace(t *testing.T) {
	loaded := bundle(t)
	ctx := context.Background()
	const reader = "spinoza-namespace-reader"
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: reader, Namespace: namespace},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"pods", "configmaps"},
			Verbs:     []string{"get", "list", "watch"},
		}},
	}
	_, err := loaded.Clientset.RbacV1().Roles(namespace).Create(ctx, role, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create role: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.RbacV1().Roles(namespace).Delete(context.Background(), role.Name, metav1.DeleteOptions{})
	})
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: reader, Namespace: namespace},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: reader, APIGroup: rbacv1.GroupName}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
	}
	_, err = loaded.Clientset.RbacV1().RoleBindings(namespace).Create(ctx, binding, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create binding: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.RbacV1().RoleBindings(namespace).Delete(context.Background(), binding.Name, metav1.DeleteOptions{})
	})
	objects := loaded.Clientset.CoreV1().ConfigMaps(namespace)
	for _, name := range []string{"seen-before", "seen-after"} {
		t.Cleanup(func() {
			_ = objects.Delete(context.Background(), name, metav1.DeleteOptions{})
		})
	}
	_, err = objects.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "seen-before"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	cfg := rest.CopyConfig(loaded.Config)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: reader}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	awaitPodsAllowed(t, cs)
	mgr := limitedManager(t, loaded, dyn, cs)

	sub, err := mgr.Subscribe(ctx, "", "v1", "configmaps", namespace, 0, nil)
	if err != nil {
		t.Fatalf("a namespace role lists directly, but browsing failed: %v", err)
	}
	t.Cleanup(sub.Close)
	if !slices.ContainsFunc(sub.Rows, func(row api.Row) bool { return row.Name == "seen-before" }) {
		t.Fatalf("rows = %+v, want the configmap that was already there", sub.Rows)
	}

	_, err = objects.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "seen-after"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(30 * time.Second)
	for {
		select {
		case ev, open := <-sub.Events:
			if !open {
				t.Fatal("the feed closed before the new configmap arrived")
			}
			if ev.Kind == "added" && ev.Row.Name == "seen-after" {
				goto elsewhere
			}
		case <-deadline:
			t.Fatal("the configmap created after subscribing never reached the namespace-scoped feed")
		}
	}

elsewhere:
	_, err = mgr.Subscribe(ctx, "", "v1", "configmaps", "default", 0, nil)
	if !apierrors.IsForbidden(err) {
		t.Fatalf("browsing another namespace gave %v, want the cluster's refusal", err)
	}
	_, err = mgr.Subscribe(ctx, "", "v1", "configmaps", "", 0, nil)
	if !errors.Is(err, resources.ErrNamespaceNeeded) {
		t.Fatalf("browsing every namespace gave %v, want to be told to pick one", err)
	}
}
