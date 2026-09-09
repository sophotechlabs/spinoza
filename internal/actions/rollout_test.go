package actions

import (
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var replicaSetGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}

func undoClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{
		deploymentGVR: "DeploymentList",
		replicaSetGVR: "ReplicaSetList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func undoTemplate(image, hash string) map[string]any {
	labels := map[string]any{"app": "web"}
	if hash != "" {
		labels["pod-template-hash"] = hash
	}
	return map[string]any{
		"metadata": map[string]any{"labels": labels},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{"name": "app", "image": image},
			},
		},
	}
}

func undoDeployment() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      "web",
			"namespace": "shop",
		},
		"spec": map[string]any{
			"replicas": int64(2),
			"template": undoTemplate("nginx:2.0", ""),
		},
	}}
}

func undoReplicaSet(name, revision, image string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "shop",
			"annotations": map[string]any{
				"deployment.kubernetes.io/revision": revision,
			},
			"ownerReferences": []any{
				map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "web"},
			},
		},
		"spec": map[string]any{
			"replicas": int64(2),
			"template": undoTemplate(image, name),
		},
	}}
}

func undoWorld() []runtime.Object {
	return []runtime.Object{
		undoDeployment(),
		undoReplicaSet("web-1", "1", "nginx:1.0"),
		undoReplicaSet("web-2", "2", "nginx:2.0"),
	}
}

func liveImage(t *testing.T, client *dynamicfake.FakeDynamicClient) string {
	t.Helper()
	containers, found, err := unstructured.NestedSlice(readDeployment(t, client).Object, "spec", "template", "spec", "containers")
	if !found || err != nil || len(containers) != 1 {
		t.Fatalf("containers: found = %v err = %v containers = %v", found, err, containers)
	}
	container, ok := containers[0].(map[string]any)
	if !ok {
		t.Fatalf("container = %v, want a map", containers[0])
	}
	image, ok := container["image"].(string)
	if !ok {
		t.Fatalf("image = %v, want a string", container["image"])
	}
	return image
}

func TestUndoWithoutARevisionSaysWhatItNeeds(t *testing.T) {
	client := undoClient(undoWorld()...)
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(t.Context(), Request{Ref: deploymentRef(), Action: Undo}, stamp)

	if !errors.Is(err, errNoRevision) {
		t.Fatalf("error = %v, want it to ask for a revision", err)
	}
	if len(*seen) != 0 {
		t.Fatalf("sent %d patches, want none", len(*seen))
	}
}

func TestUndoToARevisionThatIsNotThereNamesTheNumber(t *testing.T) {
	client := undoClient(undoWorld()...)
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(t.Context(), Request{Ref: deploymentRef(), Action: Undo, Revision: 7}, stamp)

	if err == nil {
		t.Fatal("expected a missing revision to be refused")
	}
	if !strings.Contains(err.Error(), "7") {
		t.Fatalf("error = %q, want it to name the revision", err.Error())
	}
	if len(*seen) != 0 {
		t.Fatalf("sent %d patches, want none", len(*seen))
	}
}

func TestAnUndoDryRunTouchesNothing(t *testing.T) {
	client := undoClient(undoWorld()...)
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(t.Context(), Request{Ref: deploymentRef(), Action: Undo, Revision: 1, DryRun: true}, stamp)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}

	if !result.DryRun {
		t.Fatal("dryRun = false, want the result to say it only planned")
	}
	if !strings.Contains(result.Message, "revision 1") {
		t.Fatalf("message = %q, want it to name the revision", result.Message)
	}
	if len(*seen) != 0 {
		t.Fatalf("sent %d patches, want none", len(*seen))
	}
	if image := liveImage(t, client); image != "nginx:2.0" {
		t.Fatalf("image = %q, want the deployment left alone", image)
	}
}

func TestUndoPutsBackThePodTemplateOfThatRevision(t *testing.T) {
	client := undoClient(undoWorld()...)
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(t.Context(), Request{Ref: deploymentRef(), Action: Undo, Revision: 1}, stamp)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}

	if result.Action != string(Undo) {
		t.Fatalf("action = %q, want undo", result.Action)
	}
	if !strings.Contains(result.Message, "revision 1") || !strings.Contains(result.Message, "web") {
		t.Fatalf("message = %q, want it to name the workload and the revision", result.Message)
	}
	if len(*seen) != 1 {
		t.Fatalf("sent %d patches, want 1", len(*seen))
	}
	if (*seen)[0].subresource != "" {
		t.Fatalf("subresource = %q, want the object itself", (*seen)[0].subresource)
	}
	if strings.Contains((*seen)[0].body, "pod-template-hash") {
		t.Fatalf("patch = %s, want the pod template hash left out", (*seen)[0].body)
	}
	if image := liveImage(t, client); image != "nginx:1.0" {
		t.Fatalf("image = %q, want the image of revision 1", image)
	}
}

func TestUndoIsRefusedForAKindThatKeepsNoRevisions(t *testing.T) {
	client := undoClient(undoWorld()...)
	service := serviceFor(client, k8sfake.NewClientset())
	ref := api.ObjectRef{Version: "v1", Resource: "configmaps", Namespace: "shop", Name: "settings"}

	_, err := service.Do(t.Context(), Request{Ref: ref, Action: Undo, Revision: 1}, stamp)

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want configmaps to be refused", err)
	}
}
