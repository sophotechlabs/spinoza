package inspect

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

var eventGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

var objectUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var ErrInvalidUID = errors.New("uid must be a kubernetes object uid")

func Events(ctx context.Context, dyn dynamic.Interface, namespace, uid string) ([]api.Event, error) {
	if uid == "" {
		return []api.Event{}, nil
	}
	if !objectUID.MatchString(uid) {
		return nil, fmt.Errorf("%w, got %q", ErrInvalidUID, uid)
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
		Type:      unstr.String(obj, "type"),
		Reason:    unstr.String(obj, "reason"),
		Message:   unstr.String(obj, "message"),
		Source:    sourceOf(obj),
		Count:     countOf(obj),
		FirstSeen: unstr.String(obj, "firstTimestamp"),
		LastSeen:  lastSeenOf(obj),
	}
}

func sourceOf(u *unstructured.Unstructured) string {
	component := unstr.String(u, "source", "component")
	if component != "" {
		return component
	}
	return unstr.String(u, "reportingComponent")
}

func countOf(u *unstructured.Unstructured) int64 {
	count := unstr.Int(u, "count")
	if count > 0 {
		return count
	}
	series := unstr.Int(u, "series", "count")
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
		v := unstr.String(obj, p...)
		if v != "" {
			return v
		}
	}
	return ""
}
