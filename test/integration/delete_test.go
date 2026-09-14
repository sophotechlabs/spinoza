//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/inspect"
)

func TestDeleteRefusesAReplacement(t *testing.T) {
	loaded := bundle(t)
	ctx := context.Background()
	objects := loaded.Clientset.CoreV1().ConfigMaps(namespace)
	t.Cleanup(func() {
		_ = objects.Delete(context.Background(), "recreated", metav1.DeleteOptions{})
	})
	old, err := objects.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "recreated"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ref := api.ObjectRef{Version: "v1", Resource: "configmaps", Namespace: namespace, Name: old.Name}
	detail, err := inspect.Get(ctx, loaded.Dynamic, ref)
	if err != nil {
		t.Fatal(err)
	}
	if detail.UID != string(old.UID) {
		t.Fatalf("inspected uid = %s, want %s", detail.UID, old.UID)
	}
	err = objects.Delete(ctx, old.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := objects.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: old.Name}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if replacement.UID == old.UID {
		t.Fatal("the apiserver reused the uid")
	}

	err = inspect.Delete(ctx, loaded.Dynamic, ref, detail.UID)

	if !errors.Is(err, inspect.ErrReplaced) {
		t.Fatalf("deleting with the stale uid gave %v, want ErrReplaced", err)
	}
	_, err = objects.Get(ctx, replacement.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("the replacement did not survive a stale delete: %v", err)
	}

	err = inspect.Delete(ctx, loaded.Dynamic, ref, string(replacement.UID))
	if err != nil {
		t.Fatalf("deleting with the current uid: %v", err)
	}
	_, err = objects.Get(ctx, replacement.Name, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("the object is still there after a delete with its own uid: %v", err)
	}
	err = inspect.Delete(ctx, loaded.Dynamic, ref, string(replacement.UID))
	if !apierrors.IsNotFound(err) {
		t.Fatalf("deleting an absent object gave %v, want not found", err)
	}
}
