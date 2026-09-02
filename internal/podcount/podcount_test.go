package podcount

import (
	"context"
	"errors"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"
)

func client() *metadatafake.FakeMetadataClient {
	scheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(scheme)
	if err != nil {
		panic(err)
	}
	return metadatafake.NewSimpleMetadataClient(scheme)
}

func pods(count int) []runtime.RawExtension {
	items := make([]runtime.RawExtension, 0, count)
	for i := range count {
		items = append(items, runtime.RawExtension{Object: &metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: "default",
			},
		}})
	}
	return items
}

func answerPages(
	client *metadatafake.FakeMetadataClient,
	pages [][]runtime.RawExtension,
	remaining *int64,
) *[]metav1ListCall {
	calls := &[]metav1ListCall{}
	index := 0
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if ok {
			*calls = append(*calls, metav1ListCall{fields: list.GetListRestrictions().Fields.String()})
		}
		page := pages[index]
		out := &metav1.List{Items: page}
		if index < len(pages)-1 {
			out.Continue = fmt.Sprintf("page-%d", index+1)
		}
		if index == 0 && remaining != nil {
			out.RemainingItemCount = remaining
		}
		index++
		return true, out, nil
	})
	return calls
}

type metav1ListCall struct {
	fields string
}

func TestARemainingEstimateWithoutAContinueTokenDoesNotInventPods(t *testing.T) {
	dyn := client()
	remaining := int64(41)
	answerPages(dyn, [][]runtime.RawExtension{pods(1)}, &remaining)

	got, err := Count(context.Background(), dyn, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Total != 1 {
		t.Fatalf("total = %d, want the item actually returned", got.Total)
	}
	if !got.Complete {
		t.Fatal("a list with no continuation was reported as partial")
	}
}

func TestANegativeRemainingCountIsIgnored(t *testing.T) {
	dyn := client()
	remaining := int64(-1)
	answerPages(dyn, [][]runtime.RawExtension{pods(1)}, &remaining)

	got, err := Count(context.Background(), dyn, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Total != 1 {
		t.Fatalf("total = %d, want 1", got.Total)
	}
}

func TestAFilteredCountPagesToTheEndRatherThanStoppingAtOne(t *testing.T) {
	dyn := client()
	calls := answerPages(dyn, [][]runtime.RawExtension{
		pods(500),
		pods(500),
		pods(7),
	}, nil)

	got, err := Count(context.Background(), dyn, "status.phase=Running")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Total != 1007 {
		t.Fatalf("total = %d, want every page counted rather than the first one stated as the answer", got.Total)
	}
	if !got.Complete {
		t.Fatal("a count that reached the last page said it was short")
	}
	if len(*calls) != 3 {
		t.Fatalf("list calls = %d, want one per page", len(*calls))
	}
	for at, call := range *calls {
		if call.fields != "status.phase=Running" {
			t.Fatalf("selector on call %d = %q, want it carried through the walk", at, call.fields)
		}
	}
}

func TestAnUnfilteredCountWalksWhenTheServerSizesNothing(t *testing.T) {
	dyn := client()
	calls := answerPages(dyn, [][]runtime.RawExtension{
		pods(1),
		pods(500),
		pods(7),
	}, nil)

	got, err := Count(context.Background(), dyn, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Total != 508 {
		t.Fatalf("total = %d, want every page counted, the probe's included", got.Total)
	}
	if !got.Complete {
		t.Fatal("a walk that reached the end was reported as partial")
	}
	if len(*calls) != 3 {
		t.Fatalf("list calls = %d, want the probe plus two pages", len(*calls))
	}
}

func TestASinglePageNeedsNoWalkAtAll(t *testing.T) {
	dyn := client()
	calls := answerPages(dyn, [][]runtime.RawExtension{pods(1)}, nil)

	got, err := Count(context.Background(), dyn, "status.phase=Failed")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Total != 1 {
		t.Fatalf("total = %d, want 1", got.Total)
	}
	if !got.Complete {
		t.Fatal("a single page was reported as partial")
	}
	if len(*calls) != 1 {
		t.Fatalf("list calls = %d, want just the probe", len(*calls))
	}
}

func TestARefusedListIsReported(t *testing.T) {
	dyn := client()
	dyn.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("pods is forbidden")
	})

	_, err := Count(context.Background(), dyn, "")

	if err == nil {
		t.Fatal("a refused list reported success")
	}
}

func TestAPageThatFailsMidWalkIsReported(t *testing.T) {
	dyn := client()
	index := 0
	dyn.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		index++
		if index == 1 {
			out := &metav1.List{Items: pods(1)}
			out.SetContinue("page-1")
			return true, out, nil
		}
		return true, nil, errors.New("the continue token expired")
	})

	_, err := Count(context.Background(), dyn, "")

	if err == nil {
		t.Fatal("a failed page reported success")
	}
}

func TestARepeatedContinuationTokenIsReported(t *testing.T) {
	dyn := client()
	dyn.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		out := &metav1.List{Items: pods(1)}
		out.SetContinue("same")
		return true, out, nil
	})

	_, err := Count(context.Background(), dyn, "")

	if !errors.Is(err, errRepeatedContinue) {
		t.Fatalf("count error = %v, want the repeated token", err)
	}
}

func TestAnUnfilteredProbeWalksRatherThanTrustingTheServersEstimate(t *testing.T) {
	dyn := client()
	remaining := int64(4000)
	calls := answerPages(dyn, [][]runtime.RawExtension{
		pods(1),
		pods(500),
		pods(7),
	}, &remaining)

	got, err := Count(context.Background(), dyn, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Total != 508 {
		t.Fatalf("total = %d, want the items actually returned", got.Total)
	}
	if !got.Complete {
		t.Fatal("a walk that reached the final page was reported as partial")
	}
	if len(*calls) != 3 {
		t.Fatalf("list calls = %d, want the estimate verified through pagination", len(*calls))
	}
}

func TestAnUnfilteredWalkStopsAtTheCeilingItAdvertises(t *testing.T) {
	dyn := client()
	pages := make([][]runtime.RawExtension, 0, maxPages+5)
	pages = append(pages, pods(1))
	for range maxPages + 4 {
		pages = append(pages, pods(500))
	}
	answerPages(dyn, pages, nil)

	got, err := Count(context.Background(), dyn, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Complete {
		t.Fatal("a count that gave up early claimed to be complete")
	}
	if got.Total != Limit() {
		t.Fatalf("total = %d, want exactly the %d it advertises", got.Total, Limit())
	}
}
