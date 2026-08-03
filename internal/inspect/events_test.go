package inspect

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

func newEvent(name string, fields map[string]any) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Event",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "flux-system",
		},
		"involvedObject": map[string]any{"uid": "pod-uid"},
	}
	maps.Copy(obj, fields)
	return &unstructured.Unstructured{Object: obj}
}

func TestEventsMapsAndSortsNewestFirst(t *testing.T) {
	older := newEvent("older", map[string]any{
		"type":           "Normal",
		"reason":         "Pulled",
		"message":        "image pulled",
		"count":          int64(2),
		"firstTimestamp": "2026-07-27T08:00:00Z",
		"lastTimestamp":  "2026-07-27T08:30:00Z",
		"source":         map[string]any{"component": "kubelet"},
	})
	newer := newEvent("newer", map[string]any{
		"type":           "Warning",
		"reason":         "BackOff",
		"message":        "restarting",
		"count":          int64(7),
		"firstTimestamp": "2026-07-27T09:00:00Z",
		"lastTimestamp":  "2026-07-27T09:30:00Z",
		"source":         map[string]any{"component": "kubelet"},
	})

	events, _ := Events(context.Background(), newClient(older, newer), "flux-system", "pod-uid")

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Reason != "BackOff" {
		t.Fatalf("first reason = %q, want BackOff (newest first)", events[0].Reason)
	}
	if events[0].Type != "Warning" {
		t.Fatalf("type = %q, want Warning", events[0].Type)
	}
	if events[0].Message != "restarting" {
		t.Fatalf("message = %q", events[0].Message)
	}
	if events[0].Source != "kubelet" {
		t.Fatalf("source = %q, want kubelet", events[0].Source)
	}
	if events[0].Count != 7 {
		t.Fatalf("count = %d, want 7", events[0].Count)
	}
	if events[0].FirstSeen != "2026-07-27T09:00:00Z" {
		t.Fatalf("firstSeen = %q", events[0].FirstSeen)
	}
	if events[0].LastSeen != "2026-07-27T09:30:00Z" {
		t.Fatalf("lastSeen = %q", events[0].LastSeen)
	}
}

func TestEventsSendsUIDFieldSelector(t *testing.T) {
	client := newClient()
	selector := ""
	client.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if ok {
			selector = list.GetListRestrictions().Fields.String()
		}
		return false, nil, nil
	})

	_, _ = Events(context.Background(), client, "flux-system", "pod-uid")

	if selector != "involvedObject.uid=pod-uid" {
		t.Fatalf("field selector = %q, want involvedObject.uid=pod-uid", selector)
	}
}

func TestEventsWithoutNamespaceListsClusterWide(t *testing.T) {
	client := newClient()
	namespace := "unset"
	client.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		namespace = action.GetNamespace()
		return false, nil, nil
	})

	_, _ = Events(context.Background(), client, "", "node-uid")

	if namespace != "" {
		t.Fatalf("namespace = %q, want empty for a cluster-wide list", namespace)
	}
}

func TestEventsEmptyWithoutUID(t *testing.T) {
	events, _ := Events(context.Background(), newClient(), "flux-system", "")
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0", len(events))
	}
}

func TestEventsReportsAListFailure(t *testing.T) {
	client := newClient()
	client.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("events is forbidden")
	})

	events, err := Events(context.Background(), client, "flux-system", "pod-uid")

	if err == nil {
		t.Fatal("a list failure was reported as an empty event list")
	}
	if !strings.Contains(err.Error(), "events is forbidden") {
		t.Fatalf("err = %v, want the reason from the apiserver", err)
	}
	if events != nil {
		t.Fatalf("events = %v, want nil alongside the error", events)
	}
}

func TestEventCountFallsBackToSeries(t *testing.T) {
	event := newEvent("series", map[string]any{
		"series":    map[string]any{"count": int64(4), "lastObservedTime": "2026-07-27T11:00:00Z"},
		"eventTime": "2026-07-27T10:00:00Z",
	})

	events, _ := Events(context.Background(), newClient(event), "flux-system", "pod-uid")

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Count != 4 {
		t.Fatalf("count = %d, want 4", events[0].Count)
	}
	if events[0].LastSeen != "2026-07-27T11:00:00Z" {
		t.Fatalf("lastSeen = %q, want the series observation time", events[0].LastSeen)
	}
}

func TestEventCountDefaultsToOne(t *testing.T) {
	event := newEvent("bare", map[string]any{
		"eventTime":          "2026-07-27T10:00:00Z",
		"reportingComponent": "kustomize-controller",
	})

	events, _ := Events(context.Background(), newClient(event), "flux-system", "pod-uid")

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Count != 1 {
		t.Fatalf("count = %d, want 1", events[0].Count)
	}
	if events[0].Source != "kustomize-controller" {
		t.Fatalf("source = %q, want the reporting component", events[0].Source)
	}
	if events[0].LastSeen != "2026-07-27T10:00:00Z" {
		t.Fatalf("lastSeen = %q, want the event time", events[0].LastSeen)
	}
}

func TestEventLastSeenEmptyWhenNoTimestamps(t *testing.T) {
	events, _ := Events(context.Background(), newClient(newEvent("bare", nil)), "flux-system", "pod-uid")

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].LastSeen != "" {
		t.Fatalf("lastSeen = %q, want empty", events[0].LastSeen)
	}
}

func TestEventsSortMixesTimestampPrecision(t *testing.T) {
	secondPrecision := newEvent("second", map[string]any{
		"reason":        "Pulled",
		"lastTimestamp": "2026-07-27T09:34:00Z",
	})
	subSecond := newEvent("sub-second", map[string]any{
		"reason":    "Scheduled",
		"eventTime": "2026-07-27T09:34:00.546384Z",
	})

	events, _ := Events(context.Background(), newClient(secondPrecision, subSecond), "flux-system", "pod-uid")

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Reason != "Scheduled" {
		t.Fatalf("first reason = %q, want Scheduled (the later sub-second stamp)", events[0].Reason)
	}
}

func TestEventsSortKeepsUnparseableStampsLast(t *testing.T) {
	good := newEvent("good", map[string]any{
		"reason":        "Pulled",
		"lastTimestamp": "2026-07-27T09:34:00Z",
	})
	bad := newEvent("bad", map[string]any{"reason": "Broken"})

	events, _ := Events(context.Background(), newClient(good, bad), "flux-system", "pod-uid")

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Reason != "Pulled" {
		t.Fatalf("first reason = %q, want Pulled", events[0].Reason)
	}
}
