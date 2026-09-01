package resources

import (
	"context"
	"slices"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/podcount"
)

func answerUnhealthyPages(client *metadatafake.FakeMetadataClient, pages []int) {
	at := 0
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if !ok {
			return false, nil, nil
		}
		if list.GetListRestrictions().Fields.String() != unhealthyPods {
			return false, nil, nil
		}
		out := &metav1.List{Items: make([]runtime.RawExtension, pages[at])}
		for one := range out.Items {
			out.Items[one] = runtime.RawExtension{Object: podObject("held")}
		}
		if at < len(pages)-1 {
			out.Continue = "there-is-more"
		}
		at++
		return true, out, nil
	})
}

func TestMoreFailingPodsThanOnePageAreCountedRatherThanCappedAtIt(t *testing.T) {
	dyn := podClient(t)
	answerUnhealthyPages(dyn, []int{500, 100})

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{podDesc()}, CountLimits{})

	if counts.Failing["/v1/pods"] != 600 {
		t.Fatalf("failing = %d, want every unhealthy pod counted rather than one page stated as the answer",
			counts.Failing["/v1/pods"])
	}
}

func answerUnhealthyEndlessly(client *metadatafake.FakeMetadataClient, perPage int) {
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if !ok {
			return false, nil, nil
		}
		if list.GetListRestrictions().Fields.String() != unhealthyPods {
			return false, nil, nil
		}
		out := &metav1.List{Items: make([]runtime.RawExtension, perPage)}
		for at := range out.Items {
			out.Items[at] = runtime.RawExtension{Object: podObject("held")}
		}
		out.Continue = "there-is-more"
		return true, out, nil
	})
}

func TestAFailingPodTallyThatStoppedAtTheCeilingSaysSo(t *testing.T) {
	dyn := podClient(t)
	answerUnhealthyEndlessly(dyn, 500)

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{podDesc()}, CountLimits{})

	if counts.Failing[podsKey] != podcount.Limit() {
		t.Fatalf("failing = %d, want the ceiling %d", counts.Failing[podsKey], podcount.Limit())
	}
	if !slices.Contains(counts.Capped, podsKey) {
		t.Fatalf("capped = %v, want %s", counts.Capped, podsKey)
	}
}

func TestAFailingPodTallyThatFinishedIsNotMarkedAsCapped(t *testing.T) {
	dyn := podClient(t)
	answerUnhealthyPages(dyn, []int{500, 100})

	counts := Count(context.Background(), dyn, []api.ResourceDescriptor{podDesc()}, CountLimits{})

	if len(counts.Capped) != 0 {
		t.Fatalf("capped = %v, want no capped counts", counts.Capped)
	}
}
