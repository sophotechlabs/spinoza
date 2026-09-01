package logs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestReleasedPodStaysSeenWhileItIsPresent(t *testing.T) {
	held := newAttachments()
	if !held.claim("web-0", func() {}) {
		t.Fatal("first claim was refused")
	}

	held.release("web-0")
	held.forgetGone([]string{"web-0"})

	if held.claim("web-0", func() {}) {
		t.Fatal("a completed pod was claimed again while it was still present")
	}
	if held.count() != 0 {
		t.Fatalf("open attachments = %d, want 0", held.count())
	}
}

func TestReleasedPodCanBeClaimedAfterItDisappears(t *testing.T) {
	held := newAttachments()
	if !held.claim("web-0", func() {}) {
		t.Fatal("first claim was refused")
	}
	held.release("web-0")

	held.forgetGone(nil)

	if !held.claim("web-0", func() {}) {
		t.Fatal("a pod that disappeared could not be claimed after returning")
	}
}

func TestOpenPodStaysClaimedWhileAbsentFromOneList(t *testing.T) {
	held := newAttachments()
	if !held.claim("web-0", func() {}) {
		t.Fatal("first claim was refused")
	}

	held.forgetGone(nil)

	if held.claim("web-0", func() {}) {
		t.Fatal("an active reader was duplicated after one list omitted its pod")
	}
	if held.count() != 1 {
		t.Fatalf("open attachments = %d, want 1", held.count())
	}
}

func TestForgetMakesAnOpenPodClaimableAgain(t *testing.T) {
	held := newAttachments()
	if !held.claim("web-0", func() {}) {
		t.Fatal("first claim was refused")
	}

	held.forget("web-0")

	if held.count() != 0 {
		t.Fatalf("open attachments = %d, want 0", held.count())
	}
	if !held.claim("web-0", func() {}) {
		t.Fatal("a forgotten failed attachment could not be retried")
	}
}

func TestConcurrentClaimsAllowOneReaderPerPod(t *testing.T) {
	held := newAttachments()
	const contenders = 128
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var wg sync.WaitGroup

	for range contenders {
		wg.Go(func() {
			<-start
			results <- held.claim("web-0", func() {})
		})
	}
	close(start)
	wg.Wait()
	close(results)

	winners := 0
	for claimed := range results {
		if claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("successful claims = %d, want 1", winners)
	}
	if held.count() != 1 {
		t.Fatalf("open attachments = %d, want 1", held.count())
	}
}

func TestConcurrentDistinctAttachmentsAreAllCountedAndReleased(t *testing.T) {
	held := newAttachments()
	const pods = 128
	start := make(chan struct{})
	var claimed sync.WaitGroup

	for id := range pods {
		claimed.Go(func() {
			<-start
			name := fmt.Sprintf("web-%03d", id)
			if !held.claim(name, func() {}) {
				t.Errorf("first claim for %s was refused", name)
			}
		})
	}
	close(start)
	claimed.Wait()

	if held.count() != pods {
		t.Fatalf("open attachments = %d, want %d", held.count(), pods)
	}

	var released sync.WaitGroup
	for id := range pods {
		released.Go(func() {
			held.release(fmt.Sprintf("web-%03d", id))
		})
	}
	released.Wait()

	if held.count() != 0 {
		t.Fatalf("open attachments = %d, want 0", held.count())
	}
}

func TestPodsAreSortedBeforeTheAttachmentCapIsApplied(t *testing.T) {
	objects := make([]k8sruntime.Object, 0, maxPods+5)
	for id := maxPods + 4; id >= 0; id-- {
		objects = append(objects, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("web-%02d", id),
			Namespace: "prod",
			Labels:    map[string]string{"app": "web"},
		}})
	}

	pods, matched, err := podsMatching(context.Background(), fake.NewClientset(objects...), Request{
		Namespace: "prod",
		Selector:  "app=web",
	})
	if err != nil {
		t.Fatalf("list matching pods: %v", err)
	}
	if matched != maxPods+5 {
		t.Fatalf("matched = %d, want %d", matched, maxPods+5)
	}
	if len(pods) != maxPods {
		t.Fatalf("selected = %d, want %d", len(pods), maxPods)
	}
	for id, pod := range pods {
		want := fmt.Sprintf("web-%02d", id)
		if pod.name != want {
			t.Errorf("pod %d = %q, want %q", id, pod.name, want)
		}
	}
}

func TestOpeningManyLogsReportsAPodListFailure(t *testing.T) {
	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("pods are forbidden")
	})

	stream, err := openMany(t.Context(), client, Request{
		Namespace: "prod",
		Selector:  "app=web",
	})

	if stream != nil {
		stream.Close()
		t.Fatal("a failed pod list opened a log stream")
	}
	if err == nil || !strings.Contains(err.Error(), "pods are forbidden") {
		t.Fatalf("error = %v, want the pod list failure", err)
	}
}

func TestPermanentLogRefusals(t *testing.T) {
	resource := schema.GroupResource{Resource: "pods"}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "none"},
		{name: "forbidden", err: apierrors.NewForbidden(resource, "web-0", errors.New("denied")), want: true},
		{name: "unauthorized", err: apierrors.NewUnauthorized("sign in"), want: true},
		{name: "not found", err: apierrors.NewNotFound(resource, "web-0"), want: true},
		{name: "starting", err: apierrors.NewBadRequest("container is creating")},
		{name: "transport", err: errors.New("connection reset")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := permanent(test.err); got != test.want {
				t.Fatalf("permanent(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}

func TestTailSharingBoundaries(t *testing.T) {
	tests := []struct {
		name string
		tail int64
		pods int
		want int64
	}{
		{name: "unset", pods: 20},
		{name: "negative", tail: -1, pods: 20, want: -1},
		{name: "single pod", tail: 6000, pods: 1, want: 6000},
		{name: "within per-pod budget", tail: 500, pods: 2, want: 500},
		{name: "split budget", tail: 6000, pods: 2, want: 2500},
		{name: "minimum", tail: 6000, pods: 1000, want: minTail},
		{name: "requested less than minimum", tail: 10, pods: 1000, want: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := share(test.tail, test.pods); got != test.want {
				t.Fatalf("share(%d, %d) = %d, want %d", test.tail, test.pods, got, test.want)
			}
		})
	}
}
