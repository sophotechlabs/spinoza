package podcount

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func client() *fake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{podsGVR: "PodList"}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds)
}

func pods(count int) []unstructured.Unstructured {
	items := make([]unstructured.Unstructured, 0, count)
	for i := range count {
		items = append(items, unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]any{"name": fmt.Sprintf("pod-%d", i), "namespace": "default"},
		}})
	}
	return items
}

func answerPages(dyn *fake.FakeDynamicClient, pages [][]unstructured.Unstructured, remaining *int64) *[]metav1ListCall {
	calls := &[]metav1ListCall{}
	index := 0
	dyn.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if ok {
			*calls = append(*calls, metav1ListCall{fields: list.GetListRestrictions().Fields.String()})
		}
		page := pages[index]
		out := &unstructured.UnstructuredList{Items: page}
		if index < len(pages)-1 {
			out.SetContinue(fmt.Sprintf("page-%d", index+1))
		}
		if index == 0 && remaining != nil {
			out.SetRemainingItemCount(remaining)
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
	answerPages(dyn, [][]unstructured.Unstructured{pods(1)}, &remaining)

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
	answerPages(dyn, [][]unstructured.Unstructured{pods(1)}, &remaining)

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
	calls := answerPages(dyn, [][]unstructured.Unstructured{
		pods(1),
		pods(500),
		pods(7),
	}, nil)

	got, err := Count(context.Background(), dyn, "status.phase=Running")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if got.Total != 507 {
		t.Fatalf("total = %d, want every page counted", got.Total)
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
	pages := make([][]unstructured.Unstructured, 0, maxPages+5)
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
	calls := answerPages(dyn, [][]unstructured.Unstructured{pods(1)}, nil)

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
			out := &unstructured.UnstructuredList{Items: pods(1)}
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
