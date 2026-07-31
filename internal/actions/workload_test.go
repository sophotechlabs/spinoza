package actions

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func readDeployment(t *testing.T, client *dynamicfake.FakeDynamicClient) *unstructured.Unstructured {
	t.Helper()
	got, err := client.Resource(deploymentGVR).
		Namespace("shop").
		Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return got
}

func TestScalePatchesTheScaleSubresource(t *testing.T) {
	client := dynClient(newDeployment(1))
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Scale, Replicas: 3}, stamp)
	if err != nil {
		t.Fatalf("scale: %v", err)
	}

	if len(*seen) != 1 {
		t.Fatalf("sent %d patches, want 1", len(*seen))
	}
	if (*seen)[0].subresource != "scale" {
		t.Fatalf("subresource = %q, want scale", (*seen)[0].subresource)
	}
	if (*seen)[0].body != `{"spec":{"replicas":3}}` {
		t.Fatalf("patch = %s", (*seen)[0].body)
	}
}

func TestScaleReachesZero(t *testing.T) {
	client := dynClient(newDeployment(2))
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Scale, Replicas: 0}, stamp)
	if err != nil {
		t.Fatalf("scale: %v", err)
	}

	if (*seen)[0].body != `{"spec":{"replicas":0}}` {
		t.Fatalf("patch = %s", (*seen)[0].body)
	}
	if !strings.Contains(result.Message, "to 0 replicas") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestScaleNamesASingleReplicaInTheSingular(t *testing.T) {
	client := dynClient(newDeployment(3))
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Scale, Replicas: 1}, stamp)
	if err != nil {
		t.Fatalf("scale: %v", err)
	}

	if !strings.Contains(result.Message, "to 1 replica.") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestScaleRejectsANegativeCount(t *testing.T) {
	client := dynClient(newDeployment(1))
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Scale, Replicas: -1}, stamp)

	if err == nil {
		t.Fatal("expected a negative replica count to be rejected")
	}
	if len(*seen) != 0 {
		t.Fatalf("sent %d patches, want none", len(*seen))
	}
}

func TestScaleRejectsAResourceWithoutAScaleSubresource(t *testing.T) {
	client := dynClient()
	service := serviceFor(client, k8sfake.NewClientset())
	ref := api.ObjectRef{Version: "v1", Resource: "configmaps", Namespace: "shop", Name: "settings"}

	_, err := service.Do(context.Background(), Request{Ref: ref, Action: Scale, Replicas: 2}, stamp)

	if err == nil {
		t.Fatal("expected configmaps to reject scale")
	}
}

func TestScalePropagatesAnAPIError(t *testing.T) {
	service := serviceFor(dynClient(), k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Scale, Replicas: 2}, stamp)

	if err == nil {
		t.Fatal("expected scaling a missing deployment to fail")
	}
}

func TestRestartStampsThePodTemplate(t *testing.T) {
	client := dynClient(newDeployment(1))
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Restart}, stamp)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}

	want := `{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"2026-07-31T12:00:00Z"}}}}}`
	if (*seen)[0].body != want {
		t.Fatalf("patch = %s", (*seen)[0].body)
	}
	if (*seen)[0].subresource != "" {
		t.Fatalf("subresource = %q, want the main resource", (*seen)[0].subresource)
	}
	if !strings.Contains(result.Message, "2026-07-31T12:00:00Z") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestRestartKeepsTheOtherTemplateAnnotations(t *testing.T) {
	client := dynClient(newDeployment(1))
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Restart}, stamp)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}

	got := readDeployment(t, client)
	annotations, _, _ := unstructured.NestedStringMap(got.Object, "spec", "template", "metadata", "annotations")
	if annotations["owner"] != "platform" {
		t.Fatalf("restart dropped the owner annotation: %v", annotations)
	}
	if annotations[restartAnnotation] == "" {
		t.Fatalf("restart annotation missing: %v", annotations)
	}
}

func TestRestartLeavesTheReplicaCountAlone(t *testing.T) {
	client := dynClient(newDeployment(4))
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Restart}, stamp)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}

	got := readDeployment(t, client)
	replicas, _, _ := unstructured.NestedInt64(got.Object, "spec", "replicas")
	if replicas != 4 {
		t.Fatalf("spec.replicas = %d, want 4", replicas)
	}
}

func TestRestartRejectsAResourceWithoutAPodTemplate(t *testing.T) {
	service := serviceFor(dynClient(), k8sfake.NewClientset())
	ref := api.ObjectRef{Group: "apps", Version: "v1", Resource: "replicasets", Namespace: "shop", Name: "web-1"}

	_, err := service.Do(context.Background(), Request{Ref: ref, Action: Restart}, stamp)

	if err == nil {
		t.Fatal("expected replicasets to reject restart")
	}
}

func TestRestartPropagatesAnAPIError(t *testing.T) {
	service := serviceFor(dynClient(), k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Restart}, stamp)

	if err == nil {
		t.Fatal("expected restarting a missing deployment to fail")
	}
}

func TestScaleTargetsAClusterScopedResourceWithoutANamespace(t *testing.T) {
	client := dynClient(newNode(false))
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: nodeRef(), Action: Cordon}, stamp)
	if err != nil {
		t.Fatalf("cordon: %v", err)
	}
}
