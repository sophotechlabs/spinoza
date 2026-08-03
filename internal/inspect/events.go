package inspect

import (
	"context"
	"fmt"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var eventGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

func Events(ctx context.Context, dyn dynamic.Interface, namespace, uid string) ([]api.Event, error) {
	if uid == "" {
		return []api.Event{}, nil
	}
	opts := metav1.ListOptions{FieldSelector: "involvedObject.uid=" + uid}
	list, err := eventsFor(dyn, namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	out := make([]api.Event, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, eventOf(&list.Items[i]))
	}
	sortEvents(out)
	return out, nil
}

func eventsFor(dyn dynamic.Interface, namespace string) dynamic.ResourceInterface {
	if namespace == "" {
		return dyn.Resource(eventGVR)
	}
	return dyn.Resource(eventGVR).Namespace(namespace)
}

func sortEvents(events []api.Event) {
	slices.SortStableFunc(events, func(left, right api.Event) int {
		return seenAt(right.LastSeen).Compare(seenAt(left.LastSeen))
	})
}

func seenAt(stamp string) time.Time {
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

func eventOf(obj *unstructured.Unstructured) api.Event {
	return api.Event{
		Type:      nestedString(obj, "type"),
		Reason:    nestedString(obj, "reason"),
		Message:   nestedString(obj, "message"),
		Source:    sourceOf(obj),
		Count:     countOf(obj),
		FirstSeen: nestedString(obj, "firstTimestamp"),
		LastSeen:  lastSeenOf(obj),
	}
}

func sourceOf(u *unstructured.Unstructured) string {
	component := nestedString(u, "source", "component")
	if component != "" {
		return component
	}
	return nestedString(u, "reportingComponent")
}

func countOf(u *unstructured.Unstructured) int64 {
	count := nestedInt(u, "count")
	if count > 0 {
		return count
	}
	series := nestedInt(u, "series", "count")
	if series > 0 {
		return series
	}
	return 1
}

func lastSeenOf(obj *unstructured.Unstructured) string {
	paths := [][]string{
		{"lastTimestamp"},
		{"series", "lastObservedTime"},
		{"eventTime"},
		{"firstTimestamp"},
	}
	for _, p := range paths {
		v := nestedString(obj, p...)
		if v != "" {
			return v
		}
	}
	return ""
}

func nestedString(u *unstructured.Unstructured, fields ...string) string {
	v, found, err := unstructured.NestedString(u.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return v
}

func nestedInt(u *unstructured.Unstructured, fields ...string) int64 {
	v, found, err := unstructured.NestedInt64(u.Object, fields...)
	if !found || err != nil {
		return 0
	}
	return v
}
