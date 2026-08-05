package resources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func countDesc(resource string) api.ResourceDescriptor {
	return api.ResourceDescriptor{Group: "apps", Version: "v1", Resource: resource, Kind: "Deployment"}
}

func countClient(t *testing.T, objs ...runtime.Object) *fake.FakeDynamicClient {
	t.Helper()
	kinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func countObject(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": name, "namespace": "default"},
	}}
}

func TestCountReportsZeroForAnEmptyType(t *testing.T) {
	counts := Count(context.Background(), countClient(t), []api.ResourceDescriptor{countDesc("deployments")}, CountLimits{})

	if counts.Counts["apps/v1/deployments"] != 0 {
		t.Fatalf("count = %d, want 0", counts.Counts["apps/v1/deployments"])
	}
}

func TestCountAddsWhatThePageLeftBehind(t *testing.T) {
	objs := []runtime.Object{countObject("a"), countObject("b"), countObject("c")}
	counts := Count(context.Background(), countClient(t, objs...), []api.ResourceDescriptor{countDesc("deployments")}, CountLimits{})

	if counts.Counts["apps/v1/deployments"] != 3 {
		t.Fatalf("count = %d, want 3", counts.Counts["apps/v1/deployments"])
	}
}

func TestCountReportsUnknownWhenTheListIsRefused(t *testing.T) {
	dyn := countClient(t)
	dyn.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("deployments is forbidden")
	})

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{countDesc("deployments")}, CountLimits{})

	if counts.Counts["apps/v1/deployments"] != countUnknown {
		t.Fatalf("count = %d, want the unknown marker so the browser does not claim it is empty", counts.Counts["apps/v1/deployments"])
	}
}

func TestCountCoversEveryTypeItWasGiven(t *testing.T) {
	descs := []api.ResourceDescriptor{countDesc("deployments"), countDesc("statefulsets")}

	counts := Count(context.Background(), countClient(t), descs, CountLimits{})

	if len(counts.Counts) != 2 {
		t.Fatalf("counts = %+v, want one entry per type", counts)
	}
}

func TestCountSaysWhyATypeCouldNotBeCounted(t *testing.T) {
	dyn := countClient(t)
	dyn.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("deployments is forbidden")
	})

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{countDesc("deployments")}, CountLimits{})

	reason := counts.Errors["apps/v1/deployments"]
	if !strings.Contains(reason, "forbidden") {
		t.Fatalf("reason = %q, want the sidebar able to say why it is unknown", reason)
	}
}

func TestCountLeavesNoReasonForATypeItCounted(t *testing.T) {
	counts := Count(context.Background(), countClient(t), []api.ResourceDescriptor{countDesc("deployments")}, CountLimits{})

	if len(counts.Errors) != 0 {
		t.Fatalf("errors = %+v, want none when every type answered", counts.Errors)
	}
}

func TestCountReasonTellsTheBudgetFromTheTypeFromTheApiserver(t *testing.T) {
	spent, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name   string
		budget context.Context
		err    error
		want   string
	}{
		{
			name:   "the whole budget ran out",
			budget: spent,
			err:    context.Canceled,
			want:   "budget for counting ran out",
		},
		{
			name:   "this type alone was too slow",
			budget: context.Background(),
			err:    context.DeadlineExceeded,
			want:   "counting took longer than",
		},
		{
			name:   "the apiserver refused",
			budget: context.Background(),
			err:    errors.New("deployments is forbidden"),
			want:   "forbidden",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := countReason(tc.budget, tc.err, CountLimits{}.orDefaults())
			if !strings.Contains(got, tc.want) {
				t.Fatalf("reason = %q, want it to mention %q", got, tc.want)
			}
		})
	}
}

func TestRemainingOfIgnoresANegativeOrAbsentCount(t *testing.T) {
	negative := int64(-4)
	if remainingOf(&negative) != 0 {
		t.Fatal("a negative remainder was added to the total")
	}
	if remainingOf(nil) != 0 {
		t.Fatal("an absent remainder was not treated as zero")
	}
}
