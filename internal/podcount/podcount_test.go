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

func TestAnUnfilteredCountTrustsTheRemainingItemCount(t *testing.T) {
	dyn := client()
	remaining := int64(41)
	answerPages(dyn, [][]runtime.RawExtension{pods(1)}, &remaining)

	got, err := Count(context.Background(), dyn, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Total != 42 {
		t.Fatalf("total = %d, want 42", got.Total)
	}
	if !got.Complete {
		t.Fatal("an exact count was reported as partial")
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

func TestAFilteredCountWalksThePagesTheApiserverGives(t *testing.T) {
	dyn := client()
	calls := answerPages(dyn, [][]runtime.RawExtension{
		pods(1),
		pods(500),
		pods(7),
	}, nil)

	got, err := Count(context.Background(), dyn, "status.phase=Running")
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
	if (*calls)[0].fields != "status.phase=Running" {
		t.Fatalf("selector = %q, want it on the probe", (*calls)[0].fields)
	}
}

func TestAFilteredCountStopsShortOfForever(t *testing.T) {
	dyn := client()
	pages := make([][]runtime.RawExtension, 0, maxPages+5)
	pages = append(pages, pods(1))
	for range maxPages + 4 {
		pages = append(pages, pods(500))
	}
	answerPages(dyn, pages, nil)

	got, err := Count(context.Background(), dyn, "status.phase=Running")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Complete {
		t.Fatal("a count that gave up early claimed to be complete")
	}
	if got.Total != Limit() {
		t.Fatalf("total = %d, want the %d it managed to read", got.Total, Limit())
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

	_, err := Count(context.Background(), dyn, "status.phase=Running")

	if err == nil {
		t.Fatal("a failed page reported success")
	}
}

func TestAnUnfilteredProbeStillTrustsTheServersCount(t *testing.T) {
	dyn := client()
	remaining := int64(4000)
	answerPages(dyn, [][]runtime.RawExtension{pods(1)}, &remaining)

	got, err := Count(context.Background(), dyn, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Total != 4001 {
		t.Fatalf("total = %d, want the page plus what the server said remained", got.Total)
	}
	if !got.Complete {
		t.Fatal("a count the server sized was reported as partial")
	}
}

func TestAFilteredCountStopsAtTheCeilingItAdvertises(t *testing.T) {
	dyn := client()
	pages := make([][]runtime.RawExtension, 40)
	for at := range pages {
		pages[at] = pods(500)
	}
	answerPages(dyn, pages, nil)

	got, err := Count(context.Background(), dyn, "status.phase=Running")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Complete {
		t.Fatal("a count that stopped early claimed to be complete")
	}
	if got.Total != Limit() {
		t.Fatalf("total = %d, want exactly the %d it advertises", got.Total, Limit())
	}
}
