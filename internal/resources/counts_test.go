package resources

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func countDesc(resource string) api.ResourceDescriptor {
	return api.ResourceDescriptor{Group: "apps", Version: "v1", Resource: resource, Kind: "Deployment"}
}

func countClient(t *testing.T, objs ...runtime.Object) *metadatafake.FakeMetadataClient {
	t.Helper()
	scheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(scheme)
	if err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return metadatafake.NewSimpleMetadataClient(scheme, objs...)
}

func countObject(name string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
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
	positive := int64(4)
	if remainingOf(&positive) != 4 {
		t.Fatal("a positive remainder was not added to the total")
	}
	negative := int64(-4)
	if remainingOf(&negative) != 0 {
		t.Fatal("a negative remainder was added to the total")
	}
	if remainingOf(nil) != 0 {
		t.Fatal("an absent remainder was not treated as zero")
	}
}

func podDesc() api.ResourceDescriptor {
	return api.ResourceDescriptor{Version: "v1", Resource: "pods", Kind: "Pod"}
}

func podClient(t *testing.T) *metadatafake.FakeMetadataClient {
	t.Helper()
	return countClient(t)
}

func podObject(name string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
}

func answerUnhealthy(client *metadatafake.FakeMetadataClient, items []runtime.Object, err error) {
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if !ok {
			return false, nil, nil
		}
		if list.GetListRestrictions().Fields.String() != unhealthyPods {
			return false, nil, nil
		}
		if err != nil {
			return true, nil, err
		}
		out := &metav1.List{}
		for _, item := range items {
			out.Items = append(out.Items, runtime.RawExtension{Object: item})
		}
		return true, out, nil
	})
}

func TestCountReportsPodsThatAreNeitherRunningNorDone(t *testing.T) {
	dyn := podClient(t)
	answerUnhealthy(dyn, []runtime.Object{podObject("crashing"), podObject("pending")}, nil)

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{podDesc()}, CountLimits{})

	if counts.Failing["/v1/pods"] != 2 {
		t.Fatalf("failing = %d, want 2", counts.Failing["/v1/pods"])
	}
}

func TestCountSaysNothingAboutFailingPodsWhenNoneAre(t *testing.T) {
	dyn := podClient(t)
	answerUnhealthy(dyn, nil, nil)

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{podDesc()}, CountLimits{})

	if counts.Failing != nil {
		t.Fatalf("failing = %v, want nothing to report", counts.Failing)
	}
}

func TestCountKeepsCountingWhenTheFailingPodProbeIsRefused(t *testing.T) {
	dyn := podClient(t)
	answerUnhealthy(dyn, nil, errors.New("pods is forbidden"))

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{podDesc()}, CountLimits{})

	if counts.Failing != nil {
		t.Fatalf("failing = %v, want the refused probe to report nothing", counts.Failing)
	}
	if _, ok := counts.Counts["/v1/pods"]; !ok {
		t.Fatal("the plain count went missing with the probe")
	}
}

func TestCountDoesNotProbeForPodsThatWereNotAskedAbout(t *testing.T) {
	dyn := countClient(t)
	listed := 0
	dyn.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		listed++
		return false, nil, nil
	})

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{countDesc("deployments")}, CountLimits{})

	if listed != 0 {
		t.Fatalf("pods were listed %d times for a catalog without pods", listed)
	}
	if counts.Failing != nil {
		t.Fatalf("failing = %v, want nothing", counts.Failing)
	}
}

func TestThePodsTallyIsMarkedAsCountedByPhase(t *testing.T) {
	dyn := podClient(t)
	answerUnhealthy(dyn, []runtime.Object{podObject("crashing"), podObject("pending")}, nil)

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{podDesc()}, CountLimits{})

	if len(counts.ByPhase) != 1 || counts.ByPhase[0] != podsKey {
		t.Fatalf("byPhase = %v, want the pods key named as counted by phase", counts.ByPhase)
	}
}

func TestNothingIsMarkedByPhaseWhenNoPodIsFailing(t *testing.T) {
	dyn := podClient(t)
	answerUnhealthy(dyn, nil, nil)

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{podDesc()}, CountLimits{})

	if len(counts.ByPhase) != 0 {
		t.Fatalf("byPhase = %v, want nothing marked", counts.ByPhase)
	}
}
