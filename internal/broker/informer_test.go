package broker

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func makePod(uid, name, ns string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:               types.UID(uid),
			Name:              name,
			Namespace:         ns,
			CreationTimestamp: metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
		},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Containers: []corev1.Container{
				{Name: "c1"},
				{Name: "c2"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Ready: true, RestartCount: 1},
				{Ready: false, RestartCount: 2},
			},
		},
	}
}

func waitEvent(t *testing.T, ch <-chan Event, match func(Event) bool) Event {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if match(ev) {
				return ev
			}
		case <-timeout:
			t.Fatal("timed out waiting for matching event")
			return Event{}
		}
	}
}

func TestPodToRowConvertsFields(t *testing.T) {
	pod := makePod("uid-1", "pod-a", "ns-a")
	row := podToRow(pod)

	if row.UID != "uid-1" {
		t.Fatalf("UID = %q, want uid-1", row.UID)
	}
	if row.Name != "pod-a" {
		t.Fatalf("Name = %q, want pod-a", row.Name)
	}
	if row.Namespace != "ns-a" {
		t.Fatalf("Namespace = %q, want ns-a", row.Namespace)
	}
	if row.Phase != "Running" {
		t.Fatalf("Phase = %q, want Running", row.Phase)
	}
	if row.Ready != "1/2" {
		t.Fatalf("Ready = %q, want 1/2", row.Ready)
	}
	if row.Restarts != 3 {
		t.Fatalf("Restarts = %d, want 3", row.Restarts)
	}
	if row.Node != "node-a" {
		t.Fatalf("Node = %q, want node-a", row.Node)
	}
	if row.CreatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("CreatedAt = %q, want 2026-01-02T03:04:05Z", row.CreatedAt)
	}
}

func TestPodFromDeleteAcceptsPod(t *testing.T) {
	pod := makePod("uid-1", "pod-a", "ns-a")
	got, ok := podFromDelete(pod)
	if !ok {
		t.Fatal("ok = false, want true for *Pod")
	}
	if got.UID != "uid-1" {
		t.Fatalf("UID = %q, want uid-1", got.UID)
	}
}

func TestPodFromDeleteAcceptsTombstoneWrappingPod(t *testing.T) {
	pod := makePod("uid-2", "pod-b", "ns-b")
	tombstone := cache.DeletedFinalStateUnknown{Key: "ns-b/pod-b", Obj: pod}
	got, ok := podFromDelete(tombstone)
	if !ok {
		t.Fatal("ok = false, want true for tombstone wrapping *Pod")
	}
	if got.UID != "uid-2" {
		t.Fatalf("UID = %q, want uid-2", got.UID)
	}
}

func TestPodFromDeleteRejectsTombstoneWrappingNonPod(t *testing.T) {
	tombstone := cache.DeletedFinalStateUnknown{Key: "k", Obj: "not-a-pod"}
	got, ok := podFromDelete(tombstone)
	if ok {
		t.Fatal("ok = true, want false for tombstone wrapping non-Pod")
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestPodFromDeleteRejectsPlainNonPod(t *testing.T) {
	got, ok := podFromDelete("not-a-pod")
	if ok {
		t.Fatal("ok = true, want false for plain non-Pod")
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestPublishFansOutToAllSubscribers(t *testing.T) {
	i := &informer{subs: map[chan Event]struct{}{}}
	chA, cancelA := i.Subscribe()
	defer cancelA()
	chB, cancelB := i.Subscribe()
	defer cancelB()

	ev := Event{Kind: "added", Row: podToRow(makePod("uid-1", "p", "ns"))}
	i.publish(ev)

	got := <-chA
	if got.Kind != "added" {
		t.Fatalf("chA kind = %q, want added", got.Kind)
	}
	got = <-chB
	if got.Kind != "added" {
		t.Fatalf("chB kind = %q, want added", got.Kind)
	}
}

func TestPublishDropsWhenSubscriberBufferFull(t *testing.T) {
	i := &informer{subs: map[chan Event]struct{}{}}
	ch := make(chan Event)
	i.subs[ch] = struct{}{}
	i.publish(Event{Kind: "added"})
}

func TestSubscribeCancelIsIdempotent(t *testing.T) {
	i := &informer{subs: map[chan Event]struct{}{}}
	ch, cancel := i.Subscribe()

	cancel()
	_, open := <-ch
	if open {
		t.Fatal("channel still open after cancel")
	}
	cancel()
}

func TestOnAddPublishesAddedForPod(t *testing.T) {
	i := &informer{subs: map[chan Event]struct{}{}}
	ch, cancel := i.Subscribe()
	defer cancel()

	i.onAdd(makePod("uid-1", "p", "ns"))
	got := <-ch
	if got.Kind != "added" {
		t.Fatalf("kind = %q, want added", got.Kind)
	}
	if got.Row.UID != "uid-1" {
		t.Fatalf("row UID = %q, want uid-1", got.Row.UID)
	}
}

func TestOnAddIgnoresNonPod(t *testing.T) {
	i := &informer{subs: map[chan Event]struct{}{}}
	ch, cancel := i.Subscribe()
	defer cancel()

	i.onAdd("not-a-pod")
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event %v for non-pod add", ev)
	default:
	}
}

func TestOnUpdatePublishesModifiedForPod(t *testing.T) {
	i := &informer{subs: map[chan Event]struct{}{}}
	ch, cancel := i.Subscribe()
	defer cancel()

	i.onUpdate(nil, makePod("uid-1", "p", "ns"))
	got := <-ch
	if got.Kind != "modified" {
		t.Fatalf("kind = %q, want modified", got.Kind)
	}
}

func TestOnUpdateIgnoresNonPod(t *testing.T) {
	i := &informer{subs: map[chan Event]struct{}{}}
	ch, cancel := i.Subscribe()
	defer cancel()

	i.onUpdate(nil, "not-a-pod")
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event %v for non-pod update", ev)
	default:
	}
}

func TestOnDeletePublishesDeletedForPod(t *testing.T) {
	i := &informer{subs: map[chan Event]struct{}{}}
	ch, cancel := i.Subscribe()
	defer cancel()

	i.onDelete(makePod("uid-9", "p", "ns"))
	got := <-ch
	if got.Kind != "deleted" {
		t.Fatalf("kind = %q, want deleted", got.Kind)
	}
	if got.UID != "uid-9" {
		t.Fatalf("uid = %q, want uid-9", got.UID)
	}
}

func TestOnDeleteIgnoresNonPod(t *testing.T) {
	i := &informer{subs: map[chan Event]struct{}{}}
	ch, cancel := i.Subscribe()
	defer cancel()

	i.onDelete("not-a-pod")
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event %v for non-pod delete", ev)
	default:
	}
}

func TestNewInformerSnapshotReturnsConvertedRows(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := fake.NewSimpleClientset(makePod("uid-1", "pod-a", "ns-a"))
	b, err := NewInformer(ctx, cs)
	if err != nil {
		t.Fatalf("NewInformer: %v", err)
	}

	rows, _ := b.Snapshot()
	if len(rows) != 1 {
		t.Fatalf("snapshot rows = %d, want 1", len(rows))
	}
	if rows[0].UID != "uid-1" {
		t.Fatalf("row UID = %q, want uid-1", rows[0].UID)
	}
	if rows[0].Ready != "1/2" {
		t.Fatalf("row Ready = %q, want 1/2", rows[0].Ready)
	}
}

func TestNewInformerDeliversAddUpdateDeleteEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := fake.NewSimpleClientset()
	b, err := NewInformer(ctx, cs)
	if err != nil {
		t.Fatalf("NewInformer: %v", err)
	}

	ch, unsub := b.Subscribe()
	defer unsub()

	pods := cs.CoreV1().Pods("ns-a")
	created, err := pods.Create(ctx, makePod("uid-1", "pod-a", "ns-a"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create pod: %v", err)
	}
	waitEvent(t, ch, func(ev Event) bool {
		return ev.Kind == "added" && ev.Row.UID == "uid-1"
	})

	created.Spec.NodeName = "node-b"
	_, err = pods.Update(ctx, created, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("update pod: %v", err)
	}
	waitEvent(t, ch, func(ev Event) bool {
		return ev.Kind == "modified" && ev.Row.Node == "node-b"
	})

	err = pods.Delete(ctx, "pod-a", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("delete pod: %v", err)
	}
	waitEvent(t, ch, func(ev Event) bool {
		return ev.Kind == "deleted" && ev.UID == "uid-1"
	})
}

func TestNewInformerReturnsErrorWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cs := fake.NewSimpleClientset()
	_, err := NewInformer(ctx, cs)
	if err == nil {
		t.Fatal("NewInformer returned nil error for cancelled context")
	}
}
