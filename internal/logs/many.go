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
	// A workload with more pods than this is tailed in part. Every attachment is
	// its own connection to the apiserver, and the caller is told what it got.
	maxPods = 20
	// tailBudget is how many lines the whole merged stream may start with. The
	// browser keeps a buffer of this size, so asking every pod for a full tail
	// would only fill it with whichever pods answered first and throw the rest
	// away before anyone saw them.
	tailBudget = 5000
	// A pod is worth reading with at least this much history behind it.
	minTail = 50
)

// While following, pods that appear after the stream opened are picked up, so a
// rollout does not quietly stop producing output.
var resolveEvery = 5 * time.Second

// defaultContainer is the annotation kubectl reads to pick a container when a
// pod has several and nobody said which.
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
	// Nothing to read and nothing that will ever be readable is a failed request,
	// not a silent stream. A pod that is merely still starting is different: a
	// following stream waits for it rather than giving up.
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

// attachments remembers every pod that has been read, not only the ones being
// read now: a pod whose log ended must not be opened again on the next sweep, or
// its whole log would arrive a second time.
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

// forget takes a pod back out of the record entirely, for when the apiserver
// would not hand over its log yet. A pod that is still starting says so, and the
// next sweep has to be free to ask again.
func (a *attachments) forget(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.open, name)
	delete(a.seen, name)
}

// forgetGone drops pods the cluster no longer lists, so a name that comes back
// later is read again rather than ignored forever.
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
	// A pod with several containers refuses a log request that names none, so the
	// one the pod itself points at is used unless the caller chose.
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

// share splits the caller's tail across the pods being read, so a workload with
// twenty pods opens with a full buffer of all twenty rather than a full buffer
// of the three that answered first.
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

// permanent tells a refusal apart from a pod that has not started writing yet.
// The apiserver answers 400 while a container is being created, and 403 or 404
// when it will not hand the log over at all.
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
