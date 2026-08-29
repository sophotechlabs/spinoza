package inspect

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func newEvent(name string, fields map[string]any) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Event",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "flux-system",
		},
		"involvedObject": map[string]any{"uid": "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84"},
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

	events, _ := Events(context.Background(), newClient(older, newer), "flux-system", "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84")

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

	_, _ = Events(context.Background(), client, "flux-system", "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84")

	if selector != "involvedObject.uid=6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84" {
		t.Fatalf("field selector = %q, want involvedObject.uid=6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84", selector)
	}
}

func TestEventsWithoutNamespaceListsClusterWide(t *testing.T) {
	client := newClient()
	namespace := "unset"
	client.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		namespace = action.GetNamespace()
		return false, nil, nil
	})

	_, _ = Events(context.Background(), client, "", "1c3a5e70-9b8d-4f21-a3c6-7e5d4b2a1908")

	if namespace != "" {
		t.Fatalf("namespace = %q, want empty for a cluster-wide list", namespace)
	}
}

func TestEventsRefusesAUIDThatIsNotOne(t *testing.T) {
	cases := map[string]string{
		"an injected second clause": "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84,involvedObject.namespace=kube-system",
		"a name rather than a uid":  "web-59d8f",
		"a truncated uid":           "6f1c0d3e-4a2b-4c8d-9e10",
		"trailing whitespace":       "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84 ",
	}
	for name, uid := range cases {
		t.Run(name, func(t *testing.T) {
			listed := false
			client := newClient()
			client.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
				listed = true
				return false, nil, nil
			})

			_, err := Events(context.Background(), client, "flux-system", uid)

			if !errors.Is(err, ErrInvalidUID) {
				t.Fatalf("err = %v, want it refused before the selector is built", err)
			}
			if listed {
				t.Fatal("the apiserver was asked with an unvalidated field selector")
			}
		})
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

	events, err := Events(context.Background(), client, "flux-system", "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84")

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

	events, _ := Events(context.Background(), newClient(event), "flux-system", "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84")

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

	events, _ := Events(context.Background(), newClient(event), "flux-system", "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84")

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
	events, _ := Events(context.Background(), newClient(newEvent("bare", nil)), "flux-system", "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84")

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

	events, _ := Events(context.Background(), newClient(secondPrecision, subSecond), "flux-system", "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84")

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

	events, _ := Events(context.Background(), newClient(good, bad), "flux-system", "6f1c0d3e-4a2b-4c8d-9e10-2b7f5a6c1d84")

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Reason != "Pulled" {
		t.Fatalf("first reason = %q, want Pulled", events[0].Reason)
	}
}

func TestEventObjectReadsEitherApiShape(t *testing.T) {
	cases := []struct {
		name  string
		event map[string]any
		want  string
	}{
		{
			name:  "the core involvedObject",
			event: map[string]any{"involvedObject": map[string]any{"kind": "Pod", "name": "web-0", "namespace": "prod"}},
			want:  "Pod prod/web-0",
		},
		{
			name:  "the events.k8s.io regarding",
			event: map[string]any{"regarding": map[string]any{"kind": "Pod", "name": "web-0", "namespace": "prod"}},
			want:  "Pod prod/web-0",
		},
		{
			name:  "a cluster-scoped object",
			event: map[string]any{"involvedObject": map[string]any{"kind": "Node", "name": "node-1"}},
			want:  "Node/node-1",
		},
		{
			name: "an empty involvedObject alongside a filled regarding",
			event: map[string]any{
				"involvedObject": map[string]any{},
				"regarding":      map[string]any{"kind": "Pod", "name": "web-0"},
			},
			want: "Pod/web-0",
		},
		{name: "neither", event: map[string]any{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := &unstructured.Unstructured{Object: tc.event}

			if got := eventObjectOf(item); got != tc.want {
				t.Fatalf("object = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEventSourceJoinsWhatItKnows(t *testing.T) {
	cases := []struct {
		name  string
		event map[string]any
		want  string
	}{
		{
			name:  "component and host",
			event: map[string]any{"source": map[string]any{"component": "kubelet", "host": "node-1"}},
			want:  "kubelet on node-1",
		},
		{
			name:  "component alone",
			event: map[string]any{"source": map[string]any{"component": "kubelet"}},
			want:  "kubelet",
		},
		{
			name:  "host alone",
			event: map[string]any{"source": map[string]any{"host": "node-1"}},
			want:  "node-1",
		},
		{
			name:  "the events.k8s.io reporting fields",
			event: map[string]any{"reportingComponent": "kustomize-controller", "reportingInstance": "flux-system"},
			want:  "kustomize-controller on flux-system",
		},
		{name: "nothing at all", event: map[string]any{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := &unstructured.Unstructured{Object: tc.event}

			if got := eventSourceOf(item); got != tc.want {
				t.Fatalf("source = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEventCountReadsTheDeprecatedField(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{"deprecatedCount": int64(7)}}

	if got := eventCountOf(item); got != 7 {
		t.Fatalf("count = %d, want 7", got)
	}
}

func TestFirstOfHasNothingToReturn(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{}}

	if got := firstOf(item, []string{"missing"}, []string{"absent"}); got != "" {
		t.Fatalf("found = %q, want empty", got)
	}
}

func TestEventsAtTheSameInstantKeepOneOrder(t *testing.T) {
	same := "2026-08-29T12:00:00Z"
	build := func() []api.Event {
		return []api.Event{
			{Reason: "Pulled", Source: "kubelet", Message: "c", LastSeen: same},
			{Reason: "Created", Source: "kubelet", Message: "a", LastSeen: same},
			{Reason: "Started", Source: "kubelet", Message: "b", LastSeen: same},
			{Reason: "Killing", Source: "kubelet", Message: "d", LastSeen: same},
		}
	}

	first := build()
	sortEvents(first)

	for _, order := range [][]int{{3, 2, 1, 0}, {1, 3, 0, 2}, {2, 0, 3, 1}} {
		shuffled := build()
		mixed := make([]api.Event, 0, len(order))
		for _, at := range order {
			mixed = append(mixed, shuffled[at])
		}
		sortEvents(mixed)
		if !slices.Equal(reasons(mixed), reasons(first)) {
			t.Fatalf(
				"the same events arrived in a different order and came out %v, not %v; "+
					"a tie on lastSeen must not leave the order to whatever the map handed us",
				reasons(mixed), reasons(first),
			)
		}
	}
}

func reasons(events []api.Event) []string {
	out := make([]string, 0, len(events))
	for _, one := range events {
		out = append(out, one.Reason)
	}
	return out
}

func TestTheNewestEventStillComesFirst(t *testing.T) {
	events := []api.Event{
		{Reason: "Older", LastSeen: "2026-08-29T11:00:00Z"},
		{Reason: "Newer", LastSeen: "2026-08-29T12:00:00Z"},
	}

	sortEvents(events)

	if events[0].Reason != "Newer" {
		t.Fatalf("first = %q, want the newest event", events[0].Reason)
	}
}
