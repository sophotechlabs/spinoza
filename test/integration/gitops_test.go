//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const gitopsTimeout = 2 * time.Minute

var applyForce = true

var (
	crdGVR = schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	applicationGVR = schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}
)

func applicationCRD() *unstructured.Unstructured {
	open := map[string]any{"type": "object", "x-kubernetes-preserve-unknown-fields": true}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": "applications.argoproj.io"},
		"spec": map[string]any{
			"group": "argoproj.io",
			"names": map[string]any{
				"kind":     "Application",
				"listKind": "ApplicationList",
				"plural":   "applications",
				"singular": "application",
			},
			"scope": "Namespaced",
			"versions": []any{map[string]any{
				"name":    "v1alpha1",
				"served":  true,
				"storage": true,
				"schema": map[string]any{"openAPIV3Schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"spec":      open,
						"status":    open,
						"operation": open,
					},
				}},
			}},
		},
	}}
}

func installApplicationCRD(t *testing.T, dyn dynamic.Interface) {
	t.Helper()
	ctx := context.Background()
	awaitAbsent(t, dyn)
	_, err := dyn.Resource(crdGVR).Create(ctx, applicationCRD(), metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("install the application crd: %v", err)
	}
	t.Cleanup(func() {
		_ = dyn.Resource(crdGVR).
			Delete(context.Background(), "applications.argoproj.io", metav1.DeleteOptions{})
	})
	awaitKind(t, dyn)
}

func awaitAbsent(t *testing.T, dyn dynamic.Interface) {
	t.Helper()
	deadline := time.Now().Add(gitopsTimeout)
	for time.Now().Before(deadline) {
		live, err := dyn.Resource(crdGVR).
			Get(context.Background(), "applications.argoproj.io", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return
		}
		if err == nil && live.GetDeletionTimestamp() == nil {
			return
		}
		time.Sleep(pollInterval)
	}
	t.Fatal("the application crd from an earlier test never finished going away")
}

func awaitKind(t *testing.T, dyn dynamic.Interface) {
	t.Helper()
	deadline := time.Now().Add(gitopsTimeout)
	for time.Now().Before(deadline) {
		_, err := dyn.Resource(applicationGVR).Namespace(namespace).
			List(context.Background(), metav1.ListOptions{})
		if err == nil {
			return
		}
		time.Sleep(pollInterval)
	}
	t.Fatal("the application kind never became servable")
}

func newApplicationObject(name string, status map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec": map[string]any{
			"project": "default",
			"source": map[string]any{
				"repoURL":        "https://example.test/apps",
				"path":           "podinfo",
				"targetRevision": "main",
			},
			"destination": map[string]any{"server": "https://kubernetes.default.svc", "namespace": namespace},
			"syncPolicy":  map[string]any{"automated": map[string]any{"prune": true, "selfHeal": true}},
		},
		"status": status,
	}}
}

func applyApplication(t *testing.T, dyn dynamic.Interface, obj *unstructured.Unstructured) {
	t.Helper()
	ctx := context.Background()
	name := obj.GetName()
	_, err := dyn.Resource(applicationGVR).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create application %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = dyn.Resource(applicationGVR).Namespace(namespace).
			Delete(context.Background(), name, metav1.DeleteOptions{})
	})
	written, err := dyn.Resource(applicationGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read %s back: %v", name, err)
	}
	status, found, _ := unstructured.NestedMap(obj.Object, "status")
	if !found {
		return
	}
	_ = unstructured.SetNestedMap(written.Object, status, "status")
	_, err = dyn.Resource(applicationGVR).Namespace(namespace).
		Update(ctx, written, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("write the status of %s: %v", name, err)
	}
}

func appRef(name string) api.ObjectRef {
	return api.ObjectRef{
		Group:     "argoproj.io",
		Version:   "v1alpha1",
		Resource:  "applications",
		Namespace: namespace,
		Name:      name,
	}
}

func gitopsManager(t *testing.T, loaded *kube.Bundle) *resources.Manager {
	t.Helper()
	loaded.Discovery.Invalidate()
	return manager(t, loaded)
}

func resourceNamed(app api.GitopsApp, name string) (api.GitopsResource, bool) {
	for _, one := range app.Resources {
		if one.Name == name {
			return one, true
		}
	}
	return api.GitopsResource{}, false
}

func issueTitled(app api.GitopsApp, title string) (api.GitopsIssue, bool) {
	for _, one := range app.Issues {
		if one.Title == title {
			return one, true
		}
	}
	return api.GitopsIssue{}, false
}

// what a real api server does with the patches this package writes

func TestTheApplicationCRDTakesAStatusMergePatch(t *testing.T) {
	loaded := bundle(t)
	installApplicationCRD(t, loaded.Dynamic)
	applyApplication(t, loaded.Dynamic, newApplicationObject("terminating", map[string]any{
		"operationState": map[string]any{"phase": "Running"},
	}))
	mgr := gitopsManager(t, loaded)

	_, err := mgr.ArgoAction(context.Background(), appRef("terminating"), argocd.Request{Action: argocd.Terminate})
	if err != nil {
		t.Fatalf("terminate: %v", err)
	}

	live, getErr := loaded.Dynamic.Resource(applicationGVR).Namespace(namespace).
		Get(context.Background(), "terminating", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("read back: %v", getErr)
	}
	phase, _, _ := unstructured.NestedString(live.Object, "status", "operationState", "phase")
	if phase != "Terminating" {
		t.Fatalf("phase = %q, want Terminating: the crd rejected a status merge patch", phase)
	}
}

func TestSuspendKeepsThePolicyTheApplicationHad(t *testing.T) {
	loaded := bundle(t)
	installApplicationCRD(t, loaded.Dynamic)
	applyApplication(t, loaded.Dynamic, newApplicationObject("automated", nil))
	mgr := gitopsManager(t, loaded)

	_, err := mgr.ArgoAction(context.Background(), appRef("automated"), argocd.Request{Action: argocd.Suspend})
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}

	live, _ := loaded.Dynamic.Resource(applicationGVR).Namespace(namespace).
		Get(context.Background(), "automated", metav1.GetOptions{})
	automated, found, _ := unstructured.NestedMap(live.Object, "spec", "syncPolicy", "automated")
	if !found {
		t.Fatal("suspend removed the automated block instead of switching it off")
	}
	if automated["enabled"] != false {
		t.Fatalf("enabled = %v, want false", automated["enabled"])
	}
	if automated["prune"] != true || automated["selfHeal"] != true {
		t.Fatalf("automated = %v, want prune and selfHeal kept", automated)
	}
}

func TestASyncWritesTheOptionsTheApiServerKeeps(t *testing.T) {
	loaded := bundle(t)
	installApplicationCRD(t, loaded.Dynamic)
	applyApplication(t, loaded.Dynamic, newApplicationObject("synced", nil))
	mgr := gitopsManager(t, loaded)
	req := argocd.Request{Action: argocd.Sync, Prune: true, ServerSide: true, Force: true}

	_, err := mgr.ArgoAction(context.Background(), appRef("synced"), req)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	live, _ := loaded.Dynamic.Resource(applicationGVR).Namespace(namespace).
		Get(context.Background(), "synced", metav1.GetOptions{})
	sync, found, _ := unstructured.NestedMap(live.Object, "operation", "sync")
	if !found {
		t.Fatal("the sync wrote no operation for a controller to pick up")
	}
	if sync["prune"] != true {
		t.Fatalf("prune = %v, want true", sync["prune"])
	}
	options, _, _ := unstructured.NestedStringSlice(live.Object, "operation", "sync", "syncOptions")
	if len(options) != 1 || options[0] != "ServerSideApply=true" {
		t.Fatalf("syncOptions = %v, want ServerSideApply", options)
	}
	forced, _, _ := unstructured.NestedBool(live.Object, "operation", "sync", "syncStrategy", "hook", "force")
	if !forced {
		t.Fatal("force did not reach syncStrategy.hook, so the hooks would be skipped")
	}
}

// what the per-application view reads back off a real cluster

func TestThePerApplicationViewReadsRealDrift(t *testing.T) {
	loaded := bundle(t)
	installApplicationCRD(t, loaded.Dynamic)
	applyDeclaredDeployment(t, loaded)
	applyApplication(t, loaded.Dynamic, newApplicationObject("drifting", map[string]any{
		"sync":   map[string]any{"status": "OutOfSync"},
		"health": map[string]any{"status": "Healthy"},
		"resources": []any{map[string]any{
			"group": "apps", "version": "v1", "kind": "Deployment",
			"name": "declared", "namespace": namespace, "status": "OutOfSync",
		}},
	}))
	mgr := gitopsManager(t, loaded)

	app, err := mgr.GitopsApp(context.Background(), appRef("drifting"))
	if err != nil {
		t.Fatalf("gitops app: %v", err)
	}

	if app.Controller != api.ControllerArgo {
		t.Fatalf("controller = %q, want argocd", app.Controller)
	}
	if app.Source.Repo != "https://example.test/apps" || app.Source.SyncMode != api.SyncModeAuto {
		t.Fatalf("source = %+v, want the spec split out of the object", app.Source)
	}
	found, ok := resourceNamed(app, "declared")
	if !ok {
		t.Fatalf("resources = %+v, want the deployment the controller named", app.Resources)
	}
	if found.Resource != "deployments" {
		t.Fatalf("resource = %q, want the plural discovery resolved", found.Resource)
	}
	if len(found.Drift) != 1 || found.Drift[0].Path != "spec.replicas" {
		t.Fatalf("drift = %+v, want spec.replicas read from the live object", found.Drift)
	}
	if found.Drift[0].Declared != "1" || found.Drift[0].Live != "3" {
		t.Fatalf("drift = %+v, want 1 -> 3", found.Drift[0])
	}
}

func TestThePerApplicationViewCallsOutDriftNothingWillFix(t *testing.T) {
	loaded := bundle(t)
	installApplicationCRD(t, loaded.Dynamic)
	obj := newApplicationObject("manual", map[string]any{
		"sync": map[string]any{"status": "OutOfSync"},
	})
	_ = unstructured.SetNestedMap(obj.Object, map[string]any{}, "spec", "syncPolicy")
	applyApplication(t, loaded.Dynamic, obj)
	mgr := gitopsManager(t, loaded)

	app, err := mgr.GitopsApp(context.Background(), appRef("manual"))
	if err != nil {
		t.Fatalf("gitops app: %v", err)
	}

	if app.Source.SyncMode != api.SyncModeManual {
		t.Fatalf("syncMode = %q, want manual", app.Source.SyncMode)
	}
	if _, found := issueTitled(app, "Nothing will reconcile this"); !found {
		t.Fatalf("issues = %+v, want the one that says nothing will fix it", app.Issues)
	}
}

func TestThePerApplicationGraphHangsTheResourcesOffTheApplication(t *testing.T) {
	loaded := bundle(t)
	installApplicationCRD(t, loaded.Dynamic)
	applyDeclaredDeployment(t, loaded)
	applyApplication(t, loaded.Dynamic, newApplicationObject("graphed", map[string]any{
		"resources": []any{map[string]any{
			"group": "apps", "version": "v1", "kind": "Deployment",
			"name": "declared", "namespace": namespace, "status": "Synced",
		}},
	}))
	mgr := gitopsManager(t, loaded)

	graph, err := mgr.GitopsAppGraph(context.Background(), appRef("graphed"))
	if err != nil {
		t.Fatalf("gitops app graph: %v", err)
	}

	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 {
		t.Fatalf("graph = %+v, want the application and its one resource", graph)
	}
	if graph.Edges[0].Kind != "manages" {
		t.Fatalf("edge = %+v, want a manages edge", graph.Edges[0])
	}
}

func TestThePerApplicationViewRefusesAnObjectNoControllerApplies(t *testing.T) {
	loaded := bundle(t)
	mgr := gitopsManager(t, loaded)
	ref := api.ObjectRef{
		Group: "apps", Version: "v1", Resource: "deployments",
		Namespace: namespace, Name: "declared",
	}

	_, err := mgr.GitopsApp(context.Background(), ref)

	if err == nil {
		t.Fatal("a plain deployment was read as a gitops application")
	}
}

func applyDeclaredDeployment(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	ctx := context.Background()
	declared := `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"declared",` +
		`"namespace":"` + namespace + `"},"spec":{"replicas":1}}`
	one := int32(3)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "declared",
			Namespace: namespace,
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": declared,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "declared"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "declared"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "pause",
					Image: "registry.k8s.io/pause:3.10",
				}}},
			},
		},
	}
	_, err := loaded.Clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create the declared deployment: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.AppsV1().Deployments(namespace).
			Delete(context.Background(), "declared", metav1.DeleteOptions{})
	})
	patch := []byte(`{"spec":{"replicas":3}}`)
	_, err = loaded.Clientset.AppsV1().Deployments(namespace).
		Patch(ctx, "declared", types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		t.Fatalf("scale the declared deployment: %v", err)
	}
}

// what a server-side apply leaves behind for the drift reader

func TestAServerSideAppliedResourceNamesTheWriterThatTookAField(t *testing.T) {
	loaded := bundle(t)
	installApplicationCRD(t, loaded.Dynamic)
	applySSADeployment(t, loaded)
	applyApplication(t, loaded.Dynamic, newApplicationObject("ssa", map[string]any{
		"resources": []any{map[string]any{
			"group": "apps", "version": "v1", "kind": "Deployment",
			"name": "server-side", "namespace": namespace, "status": "OutOfSync",
		}},
	}))
	mgr := gitopsManager(t, loaded)

	app, err := mgr.GitopsApp(context.Background(), appRef("ssa"))
	if err != nil {
		t.Fatalf("gitops app: %v", err)
	}

	found, ok := resourceNamed(app, "server-side")
	if !ok {
		t.Fatalf("resources = %+v, want the deployment", app.Resources)
	}
	if !found.DriftOwners {
		t.Fatalf("resource = %+v, want the rows marked as owners rather than values", found)
	}
	if len(found.Drift) != 1 || found.Drift[0].Path != "spec.replicas" {
		t.Fatalf("drift = %+v, want the field another writer took", found.Drift)
	}
	if found.Drift[0].Declared != "argocd-controller" || found.Drift[0].Live != "kubectl-edit" {
		t.Fatalf("drift = %+v, want both managers named", found.Drift[0])
	}
}

func applySSADeployment(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	ctx := context.Background()
	manifest := []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"server-side"},` +
		`"spec":{"replicas":1,"selector":{"matchLabels":{"app":"server-side"}},` +
		`"template":{"metadata":{"labels":{"app":"server-side"}},` +
		`"spec":{"containers":[{"name":"pause","image":"registry.k8s.io/pause:3.10"}]}}}}`)
	_, err := loaded.Clientset.AppsV1().Deployments(namespace).
		Patch(ctx, "server-side", types.ApplyPatchType, manifest, metav1.PatchOptions{
			FieldManager: "argocd-controller",
			Force:        &applyForce,
		})
	if err != nil {
		t.Fatalf("server-side apply the deployment: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.AppsV1().Deployments(namespace).
			Delete(context.Background(), "server-side", metav1.DeleteOptions{})
	})
	taken := []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"server-side"},"spec":{"replicas":3}}`)
	_, err = loaded.Clientset.AppsV1().Deployments(namespace).
		Patch(ctx, "server-side", types.ApplyPatchType, taken, metav1.PatchOptions{
			FieldManager: "kubectl-edit",
			Force:        &applyForce,
		})
	if err != nil {
		t.Fatalf("take the field with another manager: %v", err)
	}
}
