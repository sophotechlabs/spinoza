package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type cluster struct {
	mu       sync.Mutex
	pods     []string
	lines    map[string][]string
	listed   int
	selector string
	hold     bool
	tail     string
}

func (apiserver *cluster) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			apiserver.serveLog(w, r)
			return
		}
		apiserver.serveList(w, r)
	}
}

func (apiserver *cluster) serveList(w http.ResponseWriter, r *http.Request) {
	apiserver.mu.Lock()
	apiserver.listed++
	apiserver.selector = r.URL.Query().Get("labelSelector")
	names := append([]string{}, apiserver.pods...)
	apiserver.mu.Unlock()

	list := corev1.PodList{}
	for _, name := range names {
		list.Items = append(list.Items, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "prod"},
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (apiserver *cluster) serveLog(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	name := parts[len(parts)-2]
	apiserver.mu.Lock()
	lines := apiserver.lines[name]
	apiserver.tail = r.URL.Query().Get("tailLines")
	apiserver.mu.Unlock()
	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
	if !apiserver.holding() {
		return
	}
	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	<-r.Context().Done()
}

func (apiserver *cluster) holdOpen() {
	apiserver.mu.Lock()
	defer apiserver.mu.Unlock()
	apiserver.hold = true
}

func (apiserver *cluster) holding() bool {
	apiserver.mu.Lock()
	defer apiserver.mu.Unlock()
	return apiserver.hold
}

func (apiserver *cluster) add(name string, lines ...string) {
	apiserver.mu.Lock()
	defer apiserver.mu.Unlock()
	apiserver.pods = append(apiserver.pods, name)
	apiserver.lines[name] = lines
}

func (apiserver *cluster) askedForTail() string {
	apiserver.mu.Lock()
	defer apiserver.mu.Unlock()
	return apiserver.tail
}

func (apiserver *cluster) lists() int {
	apiserver.mu.Lock()
	defer apiserver.mu.Unlock()
	return apiserver.listed
}

func (apiserver *cluster) askedFor() string {
	apiserver.mu.Lock()
	defer apiserver.mu.Unlock()
	return apiserver.selector
}

func newCluster() *cluster {
	return &cluster{lines: map[string][]string{}}
}

func manyClient(t *testing.T, apiserver *cluster) kubernetes.Interface {
	t.Helper()
	return clientFor(t, apiserver.handler(t))
}

func gather(t *testing.T, stream *Stream, want int) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	count := 0
	deadline := time.After(5 * time.Second)
	for count < want {
		select {
		case line, ok := <-stream.Lines:
			if !ok {
				return out
			}
			out[line.Pod] = append(out[line.Pod], line.Text)
			count++
		case <-deadline:
			t.Fatalf("timed out after %d lines: %v", count, out)
		}
	}
	return out
}

func TestEveryMatchingPodIsRead(t *testing.T) {
	apiserver := newCluster()
	apiserver.add("web-0", "from web-0")
	apiserver.add("web-1", "from web-1")
	apiserver.add("web-2", "from web-2")

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()

	found := gather(t, stream, 3)

	if len(found) != 3 {
		t.Fatalf("pods = %v, want all three read", found)
	}
	for _, name := range []string{"web-0", "web-1", "web-2"} {
		if found[name][0] != "from "+name {
			t.Fatalf("%s said %v", name, found[name])
		}
	}
}

func TestALineSaysWhichPodWroteIt(t *testing.T) {
	apiserver := newCluster()
	apiserver.add("web-0", "hello")

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{Namespace: "prod", Selector: "app=web"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()

	found := gather(t, stream, 1)

	if found["web-0"] == nil {
		t.Fatalf("lines = %v, want them tagged with the pod", found)
	}
}

func TestTheSelectorIsWhatTheApiserverIsAsked(t *testing.T) {
	apiserver := newCluster()
	apiserver.add("web-0", "hello")

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web,tier=front",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	if apiserver.askedFor() != "app=web,tier=front" {
		t.Fatalf("selector = %q", apiserver.askedFor())
	}
}

func TestASelectorThatMatchesNothingSaysSo(t *testing.T) {
	apiserver := newCluster()

	_, err := Open(t.Context(), manyClient(t, apiserver), Request{Namespace: "prod", Selector: "app=gone"})

	if err == nil {
		t.Fatal("a selector matching no pods opened a stream that would never say anything")
	}
	if !strings.Contains(err.Error(), "no pods match") {
		t.Fatalf("error = %v", err)
	}
}

func TestOnlySoManyPodsAreOpenedAtOnce(t *testing.T) {
	apiserver := newCluster()
	for i := range maxPods + 5 {
		apiserver.add(fmt.Sprintf("web-%02d", i), "hello")
	}

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{Namespace: "prod", Selector: "app=web"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()

	if stream.Attached() != maxPods {
		t.Fatalf("attached = %d, want the cap", stream.Attached())
	}
	if stream.Matched() != maxPods+5 {
		t.Fatalf("matched = %d, want every pod counted", stream.Matched())
	}
}

func TestAMergedStreamEndsWhenEveryPodIsDone(t *testing.T) {
	apiserver := newCluster()
	apiserver.add("web-0", "one")
	apiserver.add("web-1", "two")

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{Namespace: "prod", Selector: "app=web"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()

	seen := 0
	for range stream.Lines {
		seen++
	}

	if seen != 2 {
		t.Fatalf("lines = %d, want both before the channel closed", seen)
	}
}

func TestAPodThatAppearsLaterIsPickedUp(t *testing.T) {
	restore := hurryResolve(t)
	defer restore()
	apiserver := newCluster()
	apiserver.add("web-0", "from web-0")

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web",
		Follow:    true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	apiserver.add("web-1", "from web-1")

	found := gather(t, stream, 1)
	if found["web-1"] == nil {
		t.Fatalf("lines = %v, want the pod that appeared after the stream opened", found)
	}
}

func TestTheCountOfPodsFollowsTheRollout(t *testing.T) {
	restore := hurryResolve(t)
	defer restore()
	apiserver := newCluster()
	apiserver.holdOpen()
	apiserver.add("web-0", "from web-0")

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web",
		Follow:    true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)
	if stream.Attached() != 1 || stream.Matched() != 1 {
		t.Fatalf("attached %d of %d, want 1 of 1 at open", stream.Attached(), stream.Matched())
	}

	apiserver.add("web-1", "from web-1")
	gather(t, stream, 1)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if stream.Attached() == 2 && stream.Matched() == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf(
		"attached %d of %d, want the second pod counted once it is being read",
		stream.Attached(),
		stream.Matched(),
	)
}

func TestAFollowingStreamKeepsAskingWhoElseIsThere(t *testing.T) {
	restore := hurryResolve(t)
	defer restore()
	apiserver := newCluster()
	apiserver.add("web-0", "hello")

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web",
		Follow:    true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if apiserver.lists() > 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the pod list was read %d times, so a rollout would go unnoticed", apiserver.lists())
}

func TestClosingAMergedStreamStopsEveryPod(t *testing.T) {
	apiserver := newCluster()
	apiserver.add("web-0", "hello")
	apiserver.add("web-1", "hello")

	stream, err := Open(context.Background(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web",
		Follow:    true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	gather(t, stream, 2)

	stream.Close()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-stream.Lines:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("the merged channel never closed after the stream was closed")
		}
	}
}

func hurryResolve(t *testing.T) func() {
	t.Helper()
	old := resolveEvery
	resolveEvery = 20 * time.Millisecond
	return func() {
		resolveEvery = old
	}
}

func TestAPodThatIsNotReadyYetIsAskedAgain(t *testing.T) {
	restore := hurryResolve(t)
	defer restore()
	apiserver := newCluster()
	apiserver.add("web-0", "ready now")
	refusals := 0
	var mu sync.Mutex
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			mu.Lock()
			first := refusals == 0
			refusals++
			mu.Unlock()
			if first {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		apiserver.handler(t)(w, r)
	})

	stream, err := Open(t.Context(), client, Request{
		Namespace: "prod",
		Selector:  "app=web",
		Follow:    true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()

	found := gather(t, stream, 1)

	if found["web-0"] == nil {
		t.Fatalf("lines = %v, want the pod read once it was ready", found)
	}
}

func TestAPodWithSeveralContainersIsAskedAboutOneOfThem(t *testing.T) {
	asked := make(chan string, 4)
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			asked <- r.URL.Query().Get("container")
			_, _ = fmt.Fprintln(w, "hello")
			return
		}
		list := corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Name: "web-0", Namespace: "prod"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app"},
				{Name: "sidecar"},
			}},
		}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	stream, err := Open(t.Context(), client, Request{Namespace: "prod", Selector: "app=web"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	select {
	case container := <-asked:
		if container != "app" {
			t.Fatalf("container = %q, want the pod's first, since naming none is refused", container)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no log request was made")
	}
}

func TestThePodsOwnDefaultContainerWins(t *testing.T) {
	asked := make(chan string, 4)
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			asked <- r.URL.Query().Get("container")
			_, _ = fmt.Fprintln(w, "hello")
			return
		}
		list := corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "web-0",
				Namespace:   "prod",
				Annotations: map[string]string{defaultContainer: "sidecar"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}}},
		}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	stream, err := Open(t.Context(), client, Request{Namespace: "prod", Selector: "app=web"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	select {
	case container := <-asked:
		if container != "sidecar" {
			t.Fatalf("container = %q, want the one the pod points at", container)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no log request was made")
	}
}

func TestAChosenContainerBeatsThePodsDefault(t *testing.T) {
	asked := make(chan string, 4)
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			asked <- r.URL.Query().Get("container")
			_, _ = fmt.Fprintln(w, "hello")
			return
		}
		list := corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "web-0",
				Namespace:   "prod",
				Annotations: map[string]string{defaultContainer: "sidecar"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}}},
		}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	stream, err := Open(t.Context(), client, Request{
		Namespace: "prod",
		Selector:  "app=web",
		Container: "app",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	select {
	case container := <-asked:
		if container != "app" {
			t.Fatalf("container = %q, want the one that was asked for", container)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no log request was made")
	}
}

func TestEveryPodRefusingIsTheWholeRequestRefusing(t *testing.T) {
	apiserver := newCluster()
	apiserver.add("web-0", "hello")
	apiserver.add("web-1", "hello")
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"kind":"Status","message":"pods/log is forbidden","code":403}`))
			return
		}
		apiserver.handler(t)(w, r)
	})

	_, err := Open(t.Context(), client, Request{Namespace: "prod", Selector: "app=web"})

	if err == nil {
		t.Fatal("a stream nobody could read reported success and would wait forever")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error = %v, want the apiserver's reason", err)
	}
}

func TestAPodThatRefusesDoesNotStopTheOthers(t *testing.T) {
	apiserver := newCluster()
	apiserver.add("web-0", "hello")
	apiserver.add("web-1", "hello")
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "web-0") && strings.HasSuffix(r.URL.Path, "/log") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		apiserver.handler(t)(w, r)
	})

	stream, err := Open(t.Context(), client, Request{Namespace: "prod", Selector: "app=web"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()

	found := gather(t, stream, 1)

	if found["web-1"] == nil {
		t.Fatalf("lines = %v, want the pod that did answer", found)
	}
	if stream.Attached() != 1 {
		t.Fatalf("attached = %d, want only the pod actually being read", stream.Attached())
	}
}

func TestASinglePodThatIsStillStartingDoesNotFailAFollowingStream(t *testing.T) {
	restore := hurryResolve(t)
	defer restore()
	apiserver := newCluster()
	apiserver.add("web-0", "ready now")
	var mu sync.Mutex
	refused := false
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			mu.Lock()
			first := !refused
			refused = true
			mu.Unlock()
			if first {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"kind":"Status","message":"container is creating","code":400}`))
				return
			}
		}
		apiserver.handler(t)(w, r)
	})

	stream, err := Open(t.Context(), client, Request{
		Namespace: "prod",
		Selector:  "app=web",
		Follow:    true,
	})
	if err != nil {
		t.Fatalf("open: %v, want the stream to wait for the pod to start", err)
	}
	defer stream.Close()

	found := gather(t, stream, 1)
	if found["web-0"] == nil {
		t.Fatalf("lines = %v, want the pod once it started", found)
	}
}

func TestAStreamThatIsNotFollowingFailsWhenNothingCanBeRead(t *testing.T) {
	apiserver := newCluster()
	apiserver.add("web-0", "hello")
	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"kind":"Status","message":"container is creating","code":400}`))
			return
		}
		apiserver.handler(t)(w, r)
	})

	_, err := Open(t.Context(), client, Request{Namespace: "prod", Selector: "app=web"})

	if err == nil {
		t.Fatal("a one-shot read of nothing reported success")
	}
}

func TestOnePodKeepsTheWholeTail(t *testing.T) {
	apiserver := newCluster()
	apiserver.add("web-0", "hello")

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web",
		TailLines: 500,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	if apiserver.askedForTail() != "500" {
		t.Fatalf("tailLines = %q, want the whole tail for a single pod", apiserver.askedForTail())
	}
}

func TestManyPodsShareTheTailBetweenThem(t *testing.T) {
	apiserver := newCluster()
	for i := range maxPods {
		apiserver.add(fmt.Sprintf("web-%02d", i), "hello")
	}

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web",
		TailLines: 500,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	if apiserver.askedForTail() != "250" {
		t.Fatalf("tailLines = %q, want the budget split across the pods", apiserver.askedForTail())
	}
}

func TestTheTailNeverShrinksBelowSomethingUseful(t *testing.T) {
	if share(500, 1000) != minTail {
		t.Fatalf("share = %d, want a floor of %d", share(500, 1000), minTail)
	}
	if share(0, 20) != 0 {
		t.Fatalf("share = %d, want no tail when none was asked for", share(0, 20))
	}
	if share(10, 20) != 10 {
		t.Fatalf("share = %d, want a small tail left alone", share(10, 20))
	}
}

func (apiserver *cluster) remove(name string) {
	apiserver.mu.Lock()
	defer apiserver.mu.Unlock()
	apiserver.pods = slices.DeleteFunc(apiserver.pods, func(held string) bool {
		return held == name
	})
}

func waitForAttached(t *testing.T, stream *Stream, want int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for stream.Attached() != want {
		select {
		case <-deadline:
			t.Fatalf("the stream is reading %d pods, want %d", stream.Attached(), want)
		case <-time.After(time.Millisecond):
		}
	}
}

func TestAPodThatComesBackIsReadAgain(t *testing.T) {
	restore := hurryResolve(t)
	defer restore()
	apiserver := newCluster()
	apiserver.add("web-0", "first time")

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web",
		Follow:    true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	apiserver.remove("web-0")
	waitForAttached(t, stream, 0)
	apiserver.add("web-0", "second time")

	found := gather(t, stream, 1)
	if found["web-0"] == nil {
		t.Fatalf("lines = %v, want the pod read again after it came back", found)
	}
}

func TestTheCapHoldsForPodsThatTurnUpLater(t *testing.T) {
	restore := hurryResolve(t)
	defer restore()
	apiserver := newCluster()
	apiserver.holdOpen()
	for i := range maxPods {
		apiserver.add(fmt.Sprintf("web-%02d", i), "hello")
	}

	stream, err := Open(t.Context(), manyClient(t, apiserver), Request{
		Namespace: "prod",
		Selector:  "app=web",
		Follow:    true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer stream.Close()
	gather(t, stream, 1)

	for i := maxPods; i < maxPods+5; i++ {
		apiserver.add(fmt.Sprintf("web-%02d", i), "hello")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if stream.Matched() > maxPods {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if stream.Attached() > maxPods {
		t.Fatalf("attached = %d, want no more than the cap of %d", stream.Attached(), maxPods)
	}
	if stream.Matched() != maxPods+5 {
		t.Fatalf("matched = %d, want every pod counted", stream.Matched())
	}
}

func TestNoErrorIsNotAPermanentRefusal(t *testing.T) {
	if permanent(nil) {
		t.Fatal("a pod that opened fine was treated as refused")
	}
}
