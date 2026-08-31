package logs

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	maxPods    = 20
	tailBudget = 5000
	minTail    = 50
)

var resolveEvery = 5 * time.Second

const defaultContainer = "kubectl.kubernetes.io/default-container"

type podRef struct {
	name      string
	container string
}

func openMany(ctx context.Context, cs kubernetes.Interface, req Request) (*Stream, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	names, matched, err := podsMatching(streamCtx, cs, req)
	if err != nil {
		cancel()
		return nil, err
	}
	if len(names) == 0 {
		cancel()
		return nil, fmt.Errorf("no pods match %s in %s", req.Selector, req.Namespace)
	}

	req.TailLines = share(req.TailLines, len(names))
	lines := make(chan Line, lineBuffer*2)
	held := newAttachments()
	var wg sync.WaitGroup
	opened := 0
	var refused error
	for _, pod := range names {
		attachErr := attach(streamCtx, cs, req, pod, lines, held, &wg)
		if attachErr != nil {
			refused = attachErr
			continue
		}
		opened++
	}
	if opened == 0 && (!req.Follow || permanent(refused)) {
		cancel()
		return nil, refused
	}
	stream := &Stream{Lines: lines, cancel: cancel, attached: opened, matched: matched}

	safe.Go("tailing "+req.Selector+" in "+req.Namespace, func() {
		defer close(lines)
		if req.Follow {
			watch(streamCtx, cs, req, lines, held, &wg, stream)
		}
		wg.Wait()
	})

	return stream, nil
}

type attachments struct {
	mu   sync.Mutex
	open map[string]func()
	seen map[string]bool
}

func newAttachments() *attachments {
	return &attachments{open: map[string]func(){}, seen: map[string]bool{}}
}

func (a *attachments) claim(name string, stop func()) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seen[name] {
		return false
	}
	a.seen[name] = true
	a.open[name] = stop
	return true
}

func (a *attachments) release(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.open, name)
}

func (a *attachments) forget(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.open, name)
	delete(a.seen, name)
}

func (a *attachments) forgetGone(present []string) {
	alive := make(map[string]bool, len(present))
	for _, name := range present {
		alive[name] = true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for name := range a.seen {
		_, reading := a.open[name]
		if alive[name] || reading {
			continue
		}
		delete(a.seen, name)
	}
}

func (a *attachments) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.open)
}

func attach(
	ctx context.Context,
	cs kubernetes.Interface,
	req Request,
	pod podRef,
	lines chan<- Line,
	held *attachments,
	wg *sync.WaitGroup,
) error {
	name := pod.name
	one := req
	one.Name = name
	one.Selector = ""
	if one.Container == "" {
		one.Container = pod.container
	}
	podCtx, stop := context.WithCancel(ctx)
	if !held.claim(name, stop) {
		stop()
		return nil
	}
	stream, err := openOne(podCtx, cs, one)
	if err != nil {
		held.forget(name)
		stop()
		return err
	}
	wg.Add(1)
	safe.Go("reading logs from "+name, func() {
		defer wg.Done()
		defer held.release(name)
		defer stream.Close()
		for line := range stream.Lines {
			select {
			case <-ctx.Done():
				return
			case lines <- Line{Pod: name, Text: line.Text}:
			}
		}
	})
	return nil
}

func watch(
	ctx context.Context,
	cs kubernetes.Interface,
	req Request,
	lines chan<- Line,
	held *attachments,
	wg *sync.WaitGroup,
	stream *Stream,
) {
	ticker := time.NewTicker(resolveEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fresh, matched, err := podsMatching(ctx, cs, req)
			if err != nil {
				continue
			}
			held.forgetGone(namesOf(fresh))
			for _, pod := range fresh {
				if held.count() >= maxPods {
					break
				}
				_ = attach(ctx, cs, req, pod, lines, held, wg)
			}
			stream.setCounts(held.count(), matched)
		}
	}
}

func podsMatching(
	ctx context.Context,
	cs kubernetes.Interface,
	req Request,
) (pods []podRef, matched int, err error) {
	list, listErr := cs.CoreV1().Pods(req.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: req.Selector,
	})
	if listErr != nil {
		return nil, 0, listErr
	}
	found := make([]podRef, 0, len(list.Items))
	for i := range list.Items {
		found = append(found, podRef{
			name:      list.Items[i].Name,
			container: containerOf(&list.Items[i]),
		})
	}
	slices.SortFunc(found, func(left, right podRef) int {
		return strings.Compare(left.name, right.name)
	})
	if len(found) <= maxPods {
		return found, len(found), nil
	}
	return found[:maxPods], len(found), nil
}

func share(tail int64, pods int) int64 {
	if tail <= 0 || pods < 2 {
		return tail
	}
	each := max(int64(tailBudget/pods), minTail)
	if each > tail {
		return tail
	}
	return each
}

func permanent(err error) bool {
	if err == nil {
		return false
	}
	return apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) || apierrors.IsNotFound(err)
}

func containerOf(pod *corev1.Pod) string {
	wanted := pod.Annotations[defaultContainer]
	if wanted != "" {
		return wanted
	}
	if len(pod.Spec.Containers) < 2 {
		return ""
	}
	return pod.Spec.Containers[0].Name
}

func namesOf(pods []podRef) []string {
	out := make([]string, 0, len(pods))
	for _, pod := range pods {
		out = append(out, pod.name)
	}
	return out
}
