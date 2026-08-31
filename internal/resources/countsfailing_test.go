package resources

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
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
