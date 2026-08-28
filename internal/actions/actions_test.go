package actions

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var (
	deploymentGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	daemonSetGVR  = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	nodeGVR       = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	configMapGVR  = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	cronJobGVR    = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}
	jobGVR        = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
)

var stamp = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

type patchRecord struct {
	subresource string
	body        string
}

func dynClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{
		deploymentGVR: "DeploymentList",
		daemonSetGVR:  "DaemonSetList",
		nodeGVR:       "NodeList",
		configMapGVR:  "ConfigMapList",
		cronJobGVR:    "CronJobList",
		jobGVR:        "JobList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func recordPatches(client *dynamicfake.FakeDynamicClient) *[]patchRecord {
	seen := &[]patchRecord{}
	client.PrependReactor("patch", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patch, ok := action.(k8stesting.PatchAction)
		if !ok {
			return false, nil, nil
		}
		*seen = append(*seen, patchRecord{
			subresource: action.GetSubresource(),
			body:        string(patch.GetPatch()),
		})
		return false, nil, nil
	})
	return seen
}

func newDeployment(replicas int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      "web",
			"namespace": "shop",
		},
		"spec": map[string]any{
			"replicas": replicas,
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{"owner": "platform"},
				},
			},
		},
	}}
}

func newNode(unschedulable bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "worker-1"},
		"spec":       map[string]any{"unschedulable": unschedulable},
	}}
}

func deploymentRef() api.ObjectRef {
	return api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "shop",
		Name:      "web",
	}
}

func nodeRef() api.ObjectRef {
	return api.ObjectRef{Version: "v1", Resource: "nodes", Name: "worker-1"}
}

func serviceFor(client *dynamicfake.FakeDynamicClient, cs *k8sfake.Clientset) *Service {
	return newWithDelay(client, cs, time.Millisecond)
}

func TestSupported(t *testing.T) {
	cases := []struct {
		ref    api.ObjectRef
		action Action
		want   bool
	}{
		{api.ObjectRef{Group: "apps", Resource: "deployments"}, Scale, true},
		{api.ObjectRef{Group: "apps", Resource: "statefulsets"}, Scale, true},
		{api.ObjectRef{Group: "apps", Resource: "replicasets"}, Scale, true},
		{api.ObjectRef{Group: "", Resource: "replicationcontrollers"}, Scale, true},
		{api.ObjectRef{Group: "apps", Resource: "daemonsets"}, Scale, false},
		{api.ObjectRef{Group: "", Resource: "pods"}, Scale, false},
		{api.ObjectRef{Group: "apps", Resource: "deployments"}, Restart, true},
		{api.ObjectRef{Group: "apps", Resource: "statefulsets"}, Restart, true},
		{api.ObjectRef{Group: "apps", Resource: "daemonsets"}, Restart, true},
		{api.ObjectRef{Group: "apps", Resource: "replicasets"}, Restart, false},
		{api.ObjectRef{Group: "", Resource: "nodes"}, Cordon, true},
		{api.ObjectRef{Group: "", Resource: "nodes"}, Uncordon, true},
		{api.ObjectRef{Group: "", Resource: "nodes"}, Drain, true},
		{api.ObjectRef{Group: "apps", Resource: "deployments"}, Drain, false},
		{api.ObjectRef{Group: "metrics.k8s.io", Resource: "nodes"}, Cordon, false},
		{api.ObjectRef{Group: "batch", Resource: "cronjobs"}, Suspend, true},
		{api.ObjectRef{Group: "batch", Resource: "cronjobs"}, Resume, true},
		{api.ObjectRef{Group: "batch", Resource: "cronjobs"}, Trigger, true},
		{api.ObjectRef{Group: "batch", Resource: "jobs"}, Suspend, false},
		{api.ObjectRef{Group: "batch", Resource: "jobs"}, Trigger, false},
		{api.ObjectRef{Group: "batch", Resource: "cronjobs"}, Restart, false},
		{api.ObjectRef{Group: "", Resource: "nodes"}, Action("explode"), false},
	}
	for _, tc := range cases {
		got := Supported(tc.ref, tc.action)
		if got != tc.want {
			t.Fatalf("Supported(%s/%s, %s) = %v, want %v", tc.ref.Group, tc.ref.Resource, tc.action, got, tc.want)
		}
	}
}

func TestDoRejectsAMissingName(t *testing.T) {
	service := serviceFor(dynClient(), k8sfake.NewClientset())
	ref := deploymentRef()
	ref.Name = ""

	_, err := service.Do(context.Background(), Request{Ref: ref, Action: Scale}, stamp)

	if err == nil {
		t.Fatal("expected a missing name to be rejected")
	}
}

func TestDoRejectsAnUnknownAction(t *testing.T) {
	service := serviceFor(dynClient(newDeployment(1)), k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: "explode"}, stamp)

	if err == nil {
		t.Fatal("expected an unknown action to be rejected")
	}
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want it to name the action rather than the resource", err)
	}
}

func TestDoRejectsAnActionTheResourceDoesNotSupport(t *testing.T) {
	service := serviceFor(dynClient(newDeployment(1)), k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Drain}, stamp)

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestKnownCoversEveryAction(t *testing.T) {
	for _, action := range []Action{Scale, Restart, Cordon, Uncordon, Drain, Suspend, Resume, Trigger} {
		if !known(action) {
			t.Fatalf("known(%s) = false", action)
		}
	}
	if known("explode") {
		t.Fatal("known(explode) = true")
	}
}

func TestDescribeNamesTheGroupWhenThereIsOne(t *testing.T) {
	if got := describe(api.ObjectRef{Resource: "nodes"}); got != "nodes" {
		t.Fatalf("describe = %q", got)
	}
	if got := describe(api.ObjectRef{Group: "apps", Resource: "deployments"}); got != "apps/deployments" {
		t.Fatalf("describe = %q", got)
	}
}
