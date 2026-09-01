package actions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const nodeName = "worker-1"

func basePod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: name},
		Spec:       corev1.PodSpec{NodeName: nodeName},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func ownedBy(pod *corev1.Pod, kind string) *corev1.Pod {
	yes := true
	pod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       kind,
		Name:       "owner",
		Controller: &yes,
	}}
	return pod
}

func replicaSetPod(name string) *corev1.Pod {
	return ownedBy(basePod(name), "ReplicaSet")
}

func daemonSetPod(name string) *corev1.Pod {
	return ownedBy(basePod(name), "DaemonSet")
}

func staticPod(name string) *corev1.Pod {
	pod := basePod(name)
	pod.Annotations = map[string]string{corev1.MirrorPodAnnotationKey: "abc"}
	return pod
}

func barePod(name string) *corev1.Pod {
	return basePod(name)
}

func emptyDirPod(name string) *corev1.Pod {
	pod := replicaSetPod(name)
	pod.Spec.Volumes = []corev1.Volume{
		{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}}},
		{Name: "cache", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}
	return pod
}

func terminatingPod(name string) *corev1.Pod {
	pod := replicaSetPod(name)
	now := metav1.NewTime(time.Now())
	pod.DeletionTimestamp = &now
	pod.Finalizers = []string{"spinoza.test/hold"}
	return pod
}

func finishedPod(name string, phase corev1.PodPhase) *corev1.Pod {
	pod := replicaSetPod(name)
	pod.Status.Phase = phase
	return pod
}

type evictionLog struct {
	mu    sync.Mutex
	names []string
}

func (e *evictionLog) add(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.names = append(e.names, name)
}

func (e *evictionLog) all() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string{}, e.names...)
}

func recordEvictions(cs *k8sfake.Clientset, answer func(name string, attempt int) error) *evictionLog {
	log := &evictionLog{}
	attempts := map[string]int{}
	var mu sync.Mutex
	cs.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		create, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		eviction, ok := create.GetObject().(*policyv1.Eviction)
		if !ok {
			return false, nil, nil
		}
		mu.Lock()
		attempts[eviction.Name]++
		attempt := attempts[eviction.Name]
		mu.Unlock()
		if answer == nil {
			log.add(eviction.Name)
			return true, nil, nil
		}
		err := answer(eviction.Name, attempt)
		if err == nil {
			log.add(eviction.Name)
		}
		return true, nil, err
	})
	return log
}

func recordListSelector(cs *k8sfake.Clientset, seen *string) {
	cs.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if !ok {
			return false, nil, nil
		}
		*seen = list.GetListRestrictions().Fields.String()
		return false, nil, nil
	})
}

func drainRequest(force, dryRun bool) Request {
	return Request{Ref: nodeRef(), Action: Drain, Force: force, DryRun: dryRun}
}

func outcomeFor(result api.ActionResult, name string) api.PodOutcome {
	for _, pod := range result.Pods {
		if pod.Name == name {
			return pod
		}
	}
	return api.PodOutcome{}
}

func readNode(t *testing.T, client *dynamicfake.FakeDynamicClient) *unstructured.Unstructured {
	t.Helper()
	got, err := client.Resource(nodeGVR).Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read node: %v", err)
	}
	return got
}

func unschedulableOf(t *testing.T, client *dynamicfake.FakeDynamicClient) bool {
	t.Helper()
	value, _, err := unstructured.NestedBool(readNode(t, client).Object, "spec", "unschedulable")
	if err != nil {
		t.Fatalf("read unschedulable: %v", err)
	}
	return value
}

func TestCordonMarksTheNodeUnschedulable(t *testing.T) {
	client := dynClient(newNode(false))
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(context.Background(), Request{Ref: nodeRef(), Action: Cordon}, stamp)
	if err != nil {
		t.Fatalf("cordon: %v", err)
	}

	if (*seen)[0].body != `{"spec":{"unschedulable":true}}` {
		t.Fatalf("patch = %s", (*seen)[0].body)
	}
	if !unschedulableOf(t, client) {
		t.Fatal("node is still schedulable")
	}
	if result.Action != string(Cordon) {
		t.Fatalf("action = %q", result.Action)
	}
}

func TestUncordonMakesTheNodeSchedulableAgain(t *testing.T) {
	client := dynClient(newNode(true))
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(context.Background(), Request{Ref: nodeRef(), Action: Uncordon}, stamp)
	if err != nil {
		t.Fatalf("uncordon: %v", err)
	}

	if (*seen)[0].body != `{"spec":{"unschedulable":false}}` {
		t.Fatalf("patch = %s", (*seen)[0].body)
	}
	if unschedulableOf(t, client) {
		t.Fatal("node is still unschedulable")
	}
	if result.Action != string(Uncordon) {
		t.Fatalf("action = %q", result.Action)
	}
}

func TestCordonRejectsAnythingThatIsNotANode(t *testing.T) {
	service := serviceFor(dynClient(newDeployment(1)), k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: deploymentRef(), Action: Cordon}, stamp)

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestCordonPropagatesAnAPIError(t *testing.T) {
	service := serviceFor(dynClient(), k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: nodeRef(), Action: Cordon}, stamp)

	if err == nil {
		t.Fatal("expected cordoning a missing node to fail")
	}
}

func TestDrainPlanClassifiesEveryPod(t *testing.T) {
	cs := k8sfake.NewClientset(
		replicaSetPod("web"),
		daemonSetPod("logger"),
		staticPod("kube-apiserver"),
		barePod("scratch"),
		emptyDirPod("builder"),
		terminatingPod("leaving"),
		finishedPod("job-done", corev1.PodSucceeded),
		finishedPod("job-lost", corev1.PodFailed),
	)
	service := serviceFor(dynClient(newNode(false)), cs)

	result, err := service.Do(context.Background(), drainRequest(false, true), stamp)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	want := map[string]string{
		"web":            api.OutcomeEvict,
		"logger":         api.OutcomeSkipped,
		"kube-apiserver": api.OutcomeSkipped,
		"scratch":        api.OutcomeBlocked,
		"builder":        api.OutcomeBlocked,
		"leaving":        api.OutcomeSkipped,
		"job-done":       api.OutcomeSkipped,
		"job-lost":       api.OutcomeSkipped,
	}
	for name, outcome := range want {
		got := outcomeFor(result, name)
		if got.Outcome != outcome {
			t.Fatalf("%s = %q (%s), want %q", name, got.Outcome, got.Reason, outcome)
		}
	}
}

func TestDrainPlanExplainsWhyAPodIsBlocked(t *testing.T) {
	cs := k8sfake.NewClientset(barePod("scratch"), emptyDirPod("builder"))
	service := serviceFor(dynClient(newNode(false)), cs)

	result, err := service.Do(context.Background(), drainRequest(false, true), stamp)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if !strings.Contains(outcomeFor(result, "scratch").Reason, "no controller") {
		t.Fatalf("scratch reason = %q", outcomeFor(result, "scratch").Reason)
	}
	if !strings.Contains(outcomeFor(result, "builder").Reason, `"cache"`) {
		t.Fatalf("builder reason = %q", outcomeFor(result, "builder").Reason)
	}
}

func TestDrainPlanTouchesNothing(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"))
	evicted := recordEvictions(cs, nil)
	client := dynClient(newNode(false))
	patches := recordPatches(client)
	service := serviceFor(client, cs)

	result, err := service.Do(context.Background(), drainRequest(false, true), stamp)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if !result.DryRun {
		t.Fatal("result is not marked as a dry run")
	}
	if len(*patches) != 0 {
		t.Fatalf("dry run cordoned the node: %v", *patches)
	}
	if len(evicted.all()) != 0 {
		t.Fatalf("dry run evicted %v", evicted.all())
	}
}

func TestDrainPlanCountsTheOutcomes(t *testing.T) {
	cs := k8sfake.NewClientset(
		replicaSetPod("web"),
		replicaSetPod("api"),
		daemonSetPod("logger"),
		barePod("scratch"),
	)
	service := serviceFor(dynClient(newNode(false)), cs)

	result, err := service.Do(context.Background(), drainRequest(false, true), stamp)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	want := "2 pods to evict, 1 left in place, 1 blocked."
	if result.Message != want {
		t.Fatalf("message = %q, want %q", result.Message, want)
	}
}

func TestDrainRefusesWhileAPodIsBlocked(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"), barePod("scratch"))
	evicted := recordEvictions(cs, nil)
	client := dynClient(newNode(false))
	patches := recordPatches(client)
	service := serviceFor(client, cs)

	_, err := service.Do(context.Background(), drainRequest(false, false), stamp)

	if err == nil {
		t.Fatal("expected the drain to be refused")
	}
	if !strings.Contains(err.Error(), "force") {
		t.Fatalf("err = %v, want it to mention force", err)
	}
	if len(*patches) != 0 {
		t.Fatal("a refused drain still cordoned the node")
	}
	if len(evicted.all()) != 0 {
		t.Fatalf("a refused drain still evicted %v", evicted.all())
	}
}

func TestDrainWithForceEvictsTheBlockedPods(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"), barePod("scratch"), emptyDirPod("builder"))
	evicted := recordEvictions(cs, nil)
	service := serviceFor(dynClient(newNode(false)), cs)

	result, err := service.Do(context.Background(), drainRequest(true, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	if len(evicted.all()) != 3 {
		t.Fatalf("evicted %v, want all three", evicted.all())
	}
	if outcomeFor(result, "scratch").Outcome != api.OutcomeEvicted {
		t.Fatalf("scratch = %q", outcomeFor(result, "scratch").Outcome)
	}
}

func TestDrainForceStillLeavesDaemonSetAndStaticPods(t *testing.T) {
	cs := k8sfake.NewClientset(daemonSetPod("logger"), staticPod("kube-apiserver"), replicaSetPod("web"))
	evicted := recordEvictions(cs, nil)
	service := serviceFor(dynClient(newNode(false)), cs)

	_, err := service.Do(context.Background(), drainRequest(true, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	if len(evicted.all()) != 1 {
		t.Fatalf("evicted %v, want only web", evicted.all())
	}
}

func TestDrainCordonsBeforeItEvicts(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"))
	client := dynClient(newNode(false))
	cordonedFirst := true
	recordEvictions(cs, func(string, int) error {
		value, _, _ := unstructured.NestedBool(readNodeUnlocked(client).Object, "spec", "unschedulable")
		cordonedFirst = value
		return nil
	})
	service := serviceFor(client, cs)

	_, err := service.Do(context.Background(), drainRequest(false, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	if !cordonedFirst {
		t.Fatal("the node was still schedulable when the first eviction went out")
	}
}

func readNodeUnlocked(client *dynamicfake.FakeDynamicClient) *unstructured.Unstructured {
	got, err := client.Resource(nodeGVR).Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return &unstructured.Unstructured{Object: map[string]any{}}
	}
	return got
}

func TestDrainRetriesWhileABudgetBlocks(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"))
	evicted := recordEvictions(cs, func(_ string, attempt int) error {
		if attempt < 3 {
			return apierrors.NewTooManyRequests("budget", 1)
		}
		return nil
	})
	service := serviceFor(dynClient(newNode(false)), cs)

	result, err := service.Do(context.Background(), drainRequest(false, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	if len(evicted.all()) != 1 {
		t.Fatalf("evicted %v, want web after the retries", evicted.all())
	}
	if outcomeFor(result, "web").Outcome != api.OutcomeEvicted {
		t.Fatalf("web = %q", outcomeFor(result, "web").Outcome)
	}
}

func TestDrainReportsAPodABudgetKeepsBlocking(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"))
	recordEvictions(cs, func(string, int) error {
		return apierrors.NewTooManyRequests("budget", 1)
	})
	service := serviceFor(dynClient(newNode(false)), cs)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := service.Do(ctx, drainRequest(false, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	got := outcomeFor(result, "web")
	if got.Outcome != api.OutcomeBlocked {
		t.Fatalf("web = %q (%s), want blocked", got.Outcome, got.Reason)
	}
	if !strings.Contains(got.Reason, "PodDisruptionBudget") {
		t.Fatalf("reason = %q", got.Reason)
	}
	if !strings.Contains(result.Message, "still blocked") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestDrainReportsAFailedEviction(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"))
	recordEvictions(cs, func(string, int) error {
		return apierrors.NewInternalError(errors.New("webhook exploded"))
	})
	service := serviceFor(dynClient(newNode(false)), cs)

	result, err := service.Do(context.Background(), drainRequest(false, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	got := outcomeFor(result, "web")
	if got.Outcome != api.OutcomeFailed {
		t.Fatalf("web = %q, want failed", got.Outcome)
	}
	if !strings.Contains(got.Reason, "webhook exploded") {
		t.Fatalf("reason = %q", got.Reason)
	}
	if !strings.Contains(result.Message, "1 failed") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestCanceledEvictionReportsTheDrainDeadlineInsteadOfTheAPIError(t *testing.T) {
	pod := replicaSetPod("web")
	cs := k8sfake.NewClientset(pod)
	recordEvictions(cs, func(string, int) error {
		return apierrors.NewInternalError(errors.New("request interrupted"))
	})
	service := serviceFor(dynClient(newNode(false)), cs)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	outcome, reason := service.evictOne(ctx, pod)

	if outcome != api.OutcomeFailed {
		t.Fatalf("outcome = %q, want failed", outcome)
	}
	if !strings.Contains(reason, "ran out of time") {
		t.Fatalf("reason = %q, want the drain deadline", reason)
	}
}

func TestDrainLimitsConcurrentEvictions(t *testing.T) {
	const pods = evictConcurrency + 5
	plans := make([]podPlan, 0, pods)
	for index := range pods {
		pod := replicaSetPod(fmt.Sprintf("pod-%02d", index))
		plans = append(plans, podPlan{
			pod: pod,
			outcome: api.PodOutcome{
				Namespace: pod.Namespace,
				Name:      pod.Name,
				Outcome:   api.OutcomeEvict,
			},
		})
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() {
			close(release)
		})
	})
	reachedCap := make(chan struct{})
	var capOnce sync.Once
	active := 0
	maximum := 0
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		active++
		if active > maximum {
			maximum = active
		}
		if active == evictConcurrency {
			capOnce.Do(func() {
				close(reachedCap)
			})
		}
		mu.Unlock()
		<-release
		mu.Lock()
		active--
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Success"}`))
	}))
	t.Cleanup(server.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	service := newWithDelay(dynClient(newNode(false)), cs, time.Millisecond)
	done := make(chan []api.PodOutcome, 1)
	go func() {
		done <- service.evictAll(t.Context(), plans)
	}()

	select {
	case <-reachedCap:
	case <-time.After(3 * time.Second):
		t.Fatal("the eviction worker pool did not fill")
	}
	releaseOnce.Do(func() {
		close(release)
	})
	outcomes := <-done

	mu.Lock()
	gotMaximum := maximum
	mu.Unlock()
	if gotMaximum != evictConcurrency {
		t.Fatalf("maximum concurrent evictions = %d, want %d", gotMaximum, evictConcurrency)
	}
	if len(outcomes) != pods {
		t.Fatalf("outcomes = %d, want %d", len(outcomes), pods)
	}
	for _, outcome := range outcomes {
		if outcome.Outcome != api.OutcomeEvicted {
			t.Fatalf("outcome for %s = %q, want evicted", outcome.Name, outcome.Outcome)
		}
	}
}

func TestDrainTreatsAVanishedPodAsGone(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"))
	recordEvictions(cs, func(name string, _ int) error {
		return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
	})
	service := serviceFor(dynClient(newNode(false)), cs)

	result, err := service.Do(context.Background(), drainRequest(false, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	got := outcomeFor(result, "web")
	if got.Outcome != api.OutcomeSkipped {
		t.Fatalf("web = %q, want skipped", got.Outcome)
	}
	if got.Reason != "already gone" {
		t.Fatalf("reason = %q", got.Reason)
	}
}

func TestDrainAsksOnlyForTheNodesPods(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"))
	selector := ""
	recordListSelector(cs, &selector)
	recordEvictions(cs, nil)
	service := serviceFor(dynClient(newNode(false)), cs)

	_, err := service.Do(context.Background(), drainRequest(false, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	if selector != "spec.nodeName=worker-1" {
		t.Fatalf("field selector = %q", selector)
	}
}

func TestDrainPropagatesAListError(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver is unhappy")
	})
	service := serviceFor(dynClient(newNode(false)), cs)

	_, err := service.Do(context.Background(), drainRequest(false, false), stamp)

	if err == nil {
		t.Fatal("expected the list failure to surface")
	}
}

func TestDrainStopsWhenTheNodeCannotBeCordoned(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"))
	evicted := recordEvictions(cs, nil)
	service := serviceFor(dynClient(), cs)

	_, err := service.Do(context.Background(), drainRequest(false, false), stamp)

	if err == nil {
		t.Fatal("expected the cordon failure to surface")
	}
	if len(evicted.all()) != 0 {
		t.Fatalf("evicted %v after the cordon failed", evicted.all())
	}
}

func TestDrainOnAnEmptyNodeSaysSo(t *testing.T) {
	service := serviceFor(dynClient(newNode(false)), k8sfake.NewClientset())

	result, err := service.Do(context.Background(), drainRequest(false, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	if result.Message != "Cordoned. Eviction requested for 0 pods." {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestDrainReportsTheEvictedCount(t *testing.T) {
	cs := k8sfake.NewClientset(replicaSetPod("web"), replicaSetPod("api"), daemonSetPod("logger"))
	recordEvictions(cs, nil)
	service := serviceFor(dynClient(newNode(false)), cs)

	result, err := service.Do(context.Background(), drainRequest(false, false), stamp)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	want := "Cordoned. Eviction requested for 2 pods, 1 left in place."
	if result.Message != want {
		t.Fatalf("message = %q, want %q", result.Message, want)
	}
}
