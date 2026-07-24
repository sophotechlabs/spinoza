package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type informer struct {
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	lister listersv1.PodLister
	shared cache.SharedIndexInformer
}

func NewInformer(ctx context.Context, cs kubernetes.Interface) (Broker, error) {
	factory := informers.NewSharedInformerFactory(cs, 0)
	pods := factory.Core().V1().Pods()
	shared := pods.Informer()

	i := &informer{
		subs:   map[chan Event]struct{}{},
		lister: pods.Lister(),
		shared: shared,
	}

	_, err := shared.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				return
			}
			i.publish(Event{Kind: "added", Row: podToRow(pod)})
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			pod, ok := newObj.(*corev1.Pod)
			if !ok {
				return
			}
			i.publish(Event{Kind: "modified", Row: podToRow(pod)})
		},
		DeleteFunc: func(obj interface{}) {
			pod, ok := podFromDelete(obj)
			if !ok {
				return
			}
			i.publish(Event{Kind: "deleted", UID: string(pod.UID)})
		},
	})
	if err != nil {
		return nil, fmt.Errorf("add event handler: %w", err)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), shared.HasSynced) {
		return nil, fmt.Errorf("wait for cache sync: pods informer did not sync")
	}

	return i, nil
}

func (i *informer) publish(ev Event) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for ch := range i.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (i *informer) Snapshot() ([]api.PodRow, string) {
	rv := i.shared.LastSyncResourceVersion()
	pods, err := i.lister.List(labels.Everything())
	if err != nil {
		return []api.PodRow{}, rv
	}
	rows := make([]api.PodRow, 0, len(pods))
	for _, pod := range pods {
		rows = append(rows, podToRow(pod))
	}
	return rows, rv
}

func (i *informer) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	i.mu.Lock()
	i.subs[ch] = struct{}{}
	i.mu.Unlock()
	cancel := func() {
		i.mu.Lock()
		defer i.mu.Unlock()
		if _, ok := i.subs[ch]; ok {
			delete(i.subs, ch)
			close(ch)
		}
	}
	return ch, cancel
}

func podFromDelete(obj interface{}) (*corev1.Pod, bool) {
	pod, ok := obj.(*corev1.Pod)
	if ok {
		return pod, true
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		return nil, false
	}
	pod, ok = tombstone.Obj.(*corev1.Pod)
	if !ok {
		return nil, false
	}
	return pod, true
}

func podToRow(pod *corev1.Pod) api.PodRow {
	readyCount := 0
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			readyCount++
		}
	}
	total := len(pod.Spec.Containers)
	var restarts int32
	for _, status := range pod.Status.ContainerStatuses {
		restarts += status.RestartCount
	}
	return api.PodRow{
		UID:       string(pod.UID),
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Phase:     string(pod.Status.Phase),
		Ready:     fmt.Sprintf("%d/%d", readyCount, total),
		Restarts:  restarts,
		Node:      pod.Spec.NodeName,
		CreatedAt: pod.CreationTimestamp.Time.UTC().Format(time.RFC3339),
	}
}
