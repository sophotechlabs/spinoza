package debugcontainer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type stubRunner struct {
	calls   [][]string
	err     error
	onRun   func()
	refuses bool
}

func (s *stubRunner) Run(_ context.Context, args []string) error {
	s.calls = append(s.calls, args)
	if s.refuses {
		return s.err
	}
	if s.onRun != nil {
		s.onRun()
	}
	return nil
}

func (s *stubRunner) lastArgs() []string {
	if len(s.calls) == 0 {
		return nil
	}
	return s.calls[len(s.calls)-1]
}

func runningPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "monitoring", Name: "loki-0"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "loki"}, {Name: "sidecar"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func request() Request {
	return Request{Namespace: "monitoring", Pod: "loki-0", Container: "loki"}
}

func newService(t *testing.T, pod *corev1.Pod, runner *stubRunner) *Service {
	t.Helper()
	client := k8sfake.NewClientset(pod)
	if runner.onRun == nil && !runner.refuses {
		runner.onRun = func() {
			current, err := client.CoreV1().Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
			if err != nil {
				return
			}
			name := nextName(current)
			current.Spec.EphemeralContainers = append(current.Spec.EphemeralContainers, corev1.EphemeralContainer{
				EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: name},
			})
			current.Status.EphemeralContainerStatuses = append(current.Status.EphemeralContainerStatuses, corev1.ContainerStatus{
				Name:  name,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			})
			_, _ = client.CoreV1().Pods(pod.Namespace).UpdateStatus(context.Background(), current, metav1.UpdateOptions{})
			_, _ = client.CoreV1().Pods(pod.Namespace).Update(context.Background(), current, metav1.UpdateOptions{})
		}
	}
	service := NewService(runner, client, "", "p-mk1")
	service.poll = time.Millisecond
	service.timeout = 2 * time.Second
	return service
}

func TestEnsureCreatesTheFirstDebugContainer(t *testing.T) {
	runner := &stubRunner{}
	service := newService(t, runningPod(), runner)

	session, err := service.Ensure(context.Background(), request())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if session.Container != "spinoza-debug-1" {
		t.Fatalf("container = %q", session.Container)
	}
	if !session.Created {
		t.Fatal("expected created = true")
	}
	if session.Image != DefaultImage {
		t.Fatalf("image = %q", session.Image)
	}
	if session.Profile != DefaultProfile {
		t.Fatalf("profile = %q", session.Profile)
	}
}

func TestEnsurePassesTheExpectedKubectlArguments(t *testing.T) {
	runner := &stubRunner{}
	service := newService(t, runningPod(), runner)

	_, err := service.Ensure(context.Background(), request())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	joined := strings.Join(runner.lastArgs(), " ")
	for _, want := range []string{
		"debug loki-0",
		"--namespace monitoring",
		"--image " + DefaultImage,
		"--container spinoza-debug-1",
		"--profile general",
		"--attach=false",
		"--stdin",
		"--target loki",
		"--context p-mk1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

func TestEnsureReusesARunningDebugContainer(t *testing.T) {
	pod := runningPod()
	pod.Spec.EphemeralContainers = []corev1.EphemeralContainer{
		{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "spinoza-debug-1"}},
	}
	pod.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{
		{Name: "spinoza-debug-1", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
	}
	runner := &stubRunner{}
	service := newService(t, pod, runner)

	session, err := service.Ensure(context.Background(), request())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if session.Container != "spinoza-debug-1" {
		t.Fatalf("container = %q", session.Container)
	}
	if session.Created {
		t.Fatal("expected the existing container to be reused")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("kubectl ran %d times, want 0", len(runner.calls))
	}
}

func TestEnsureSkipsATerminatedDebugContainer(t *testing.T) {
	pod := runningPod()
	pod.Spec.EphemeralContainers = []corev1.EphemeralContainer{
		{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "spinoza-debug-1"}},
	}
	pod.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{
		{Name: "spinoza-debug-1", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}}},
	}
	runner := &stubRunner{}
	service := newService(t, pod, runner)

	session, err := service.Ensure(context.Background(), request())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if session.Container != "spinoza-debug-2" {
		t.Fatalf("container = %q, want the next index", session.Container)
	}
}

func TestEnsureIgnoresForeignEphemeralContainers(t *testing.T) {
	pod := runningPod()
	pod.Spec.EphemeralContainers = []corev1.EphemeralContainer{
		{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debugger-abcde"}},
	}
	pod.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{
		{Name: "debugger-abcde", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
	}
	runner := &stubRunner{}
	service := newService(t, pod, runner)

	session, err := service.Ensure(context.Background(), request())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if session.Container != "spinoza-debug-1" {
		t.Fatalf("container = %q, a kubectl-made container is not spinoza's to reuse", session.Container)
	}
}

func TestEnsureRejectsANonRunningPod(t *testing.T) {
	pod := runningPod()
	pod.Status.Phase = corev1.PodSucceeded
	service := newService(t, pod, &stubRunner{})

	_, err := service.Ensure(context.Background(), request())
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "running pod") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestEnsureRejectsAStaticPod(t *testing.T) {
	pod := runningPod()
	pod.Annotations = map[string]string{mirrorPodKey: "abc"}
	service := newService(t, pod, &stubRunner{})

	_, err := service.Ensure(context.Background(), request())
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "static pod") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestEnsureRejectsAnUnknownTargetContainer(t *testing.T) {
	service := newService(t, runningPod(), &stubRunner{})

	_, err := service.Ensure(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0", Container: "ghost"})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "no container") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestEnsureRejectsAnUnknownProfile(t *testing.T) {
	service := newService(t, runningPod(), &stubRunner{})

	req := request()
	req.Profile = "root-everything"
	_, err := service.Ensure(context.Background(), req)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "unknown debug profile") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestEnsureRejectsNamesThatCouldBeReadAsFlags(t *testing.T) {
	service := newService(t, runningPod(), &stubRunner{})

	_, err := service.Ensure(context.Background(), Request{Namespace: "monitoring", Pod: "--kubeconfig=/etc/shadow"})
	if err == nil {
		t.Fatal("a pod name that looks like a flag must be refused")
	}
}

func TestEnsureReportsAMissingPod(t *testing.T) {
	service := NewService(&stubRunner{}, k8sfake.NewClientset(), "", "")
	_, err := service.Ensure(context.Background(), request())
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestEnsureSurfacesTheRunnerFailure(t *testing.T) {
	runner := &stubRunner{refuses: true, err: errors.New("forbidden: cannot patch ephemeralcontainers")}
	service := newService(t, runningPod(), runner)

	_, err := service.Ensure(context.Background(), request())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "ephemeralcontainers") {
		t.Fatalf("message = %q, want the kubectl message verbatim", err.Error())
	}
}

func TestEnsureFailsFastOnAPullFailure(t *testing.T) {
	pod := runningPod()
	client := k8sfake.NewClientset(pod)
	runner := &stubRunner{}
	runner.onRun = func() {
		current, _ := client.CoreV1().Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		current.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{{
			Name: "spinoza-debug-1",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason:  "ImagePullBackOff",
				Message: "Back-off pulling image",
			}},
		}}
		_, _ = client.CoreV1().Pods(pod.Namespace).UpdateStatus(context.Background(), current, metav1.UpdateOptions{})
	}
	service := NewService(runner, client, "", "")
	service.poll = time.Millisecond
	service.timeout = 30 * time.Second

	started := time.Now()
	_, err := service.Ensure(context.Background(), request())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "ImagePullBackOff") {
		t.Fatalf("message = %q", err.Error())
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("a pull failure must not wait out the timeout")
	}
}

func TestEnsureReportsAContainerThatExitsImmediately(t *testing.T) {
	pod := runningPod()
	client := k8sfake.NewClientset(pod)
	runner := &stubRunner{}
	runner.onRun = func() {
		current, _ := client.CoreV1().Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		current.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{{
			Name:  "spinoza-debug-1",
			State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error"}},
		}}
		_, _ = client.CoreV1().Pods(pod.Namespace).UpdateStatus(context.Background(), current, metav1.UpdateOptions{})
	}
	service := NewService(runner, client, "", "")
	service.poll = time.Millisecond
	service.timeout = 2 * time.Second

	_, err := service.Ensure(context.Background(), request())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "exited immediately") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestEnsureTimesOutWhenTheContainerNeverStarts(t *testing.T) {
	runner := &stubRunner{onRun: func() {}}
	service := newService(t, runningPod(), runner)
	service.timeout = 30 * time.Millisecond

	_, err := service.Ensure(context.Background(), request())
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if !strings.Contains(err.Error(), "did not start") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestEnsureOmitsTheTargetWhenNoContainerIsNamed(t *testing.T) {
	runner := &stubRunner{}
	service := newService(t, runningPod(), runner)

	_, err := service.Ensure(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0"})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if strings.Contains(strings.Join(runner.lastArgs(), " "), "--target") {
		t.Fatal("no target container was named, so --target must be omitted")
	}
}

func TestSupportedProfiles(t *testing.T) {
	for _, name := range []string{"general", "sysadmin", "netadmin", "baseline", "restricted", "legacy"} {
		if !Supported(name) {
			t.Fatalf("%s should be supported", name)
		}
	}
	if Supported("root") {
		t.Fatal("unknown profiles must be refused")
	}
}

func TestNextNameCountsFromTheHighestExisting(t *testing.T) {
	pod := runningPod()
	pod.Spec.EphemeralContainers = []corev1.EphemeralContainer{
		{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "spinoza-debug-1"}},
		{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debugger-zzz"}},
		{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "spinoza-debug-7"}},
		{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "spinoza-debug-notanumber"}},
	}
	if got := nextName(pod); got != "spinoza-debug-8" {
		t.Fatalf("nextName = %q", got)
	}
}

func TestEnsureHonoursACancelledContext(t *testing.T) {
	runner := &stubRunner{onRun: func() {}}
	service := newService(t, runningPod(), runner)
	service.timeout = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	_, err := service.Ensure(ctx, request())
	if err == nil {
		t.Fatal("expected the cancellation to surface")
	}
}

func TestEnsureRejectsABadNamespaceName(t *testing.T) {
	service := newService(t, runningPod(), &stubRunner{})

	_, err := service.Ensure(context.Background(), Request{Namespace: "Not Valid", Pod: "loki-0"})
	if err == nil {
		t.Fatal("expected a refusal")
	}
}

func TestEnsureRejectsABadContainerName(t *testing.T) {
	service := newService(t, runningPod(), &stubRunner{})

	_, err := service.Ensure(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0", Container: "--flag"})
	if err == nil {
		t.Fatal("expected a refusal")
	}
}

func TestEnsureReportsAPodThatVanishesWhileWaiting(t *testing.T) {
	pod := runningPod()
	client := k8sfake.NewClientset(pod)
	runner := &stubRunner{}
	runner.onRun = func() {
		_ = client.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
	}
	service := NewService(runner, client, "", "")
	service.poll = time.Millisecond
	service.timeout = 2 * time.Second

	_, err := service.Ensure(context.Background(), request())
	if err == nil {
		t.Fatal("expected an error once the pod is gone")
	}
}

func allowingClient(t *testing.T, allowed bool, reason string) *k8sfake.Clientset {
	t.Helper()
	client := k8sfake.NewClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			create, isCreate := action.(k8stesting.CreateAction)
			if !isCreate {
				return false, nil, nil
			}
			review, isReview := create.GetObject().(*authv1.SelfSubjectAccessReview)
			if !isReview {
				return false, nil, nil
			}
			review.Status = authv1.SubjectAccessReviewStatus{Allowed: allowed, Reason: reason}
			return true, review, nil
		})
	return client
}

func TestAllowedReportsAPermittedNamespace(t *testing.T) {
	service := NewService(&stubRunner{}, allowingClient(t, true, ""), "", "")

	support := service.Allowed(context.Background(), "monitoring")
	if !support.Allowed {
		t.Fatal("expected the namespace to be allowed")
	}
	if support.Namespace != "monitoring" {
		t.Fatalf("namespace = %q", support.Namespace)
	}
}

func TestAllowedReportsARefusalWithItsReason(t *testing.T) {
	service := NewService(&stubRunner{}, allowingClient(t, false, "no RBAC policy matched"), "", "")

	support := service.Allowed(context.Background(), "kube-system")
	if support.Allowed {
		t.Fatal("expected the namespace to be refused")
	}
	if support.Reason != "no RBAC policy matched" {
		t.Fatalf("reason = %q", support.Reason)
	}
}

func TestAllowedDefaultsToPermittedWhenTheReviewItselfFails(t *testing.T) {
	client := k8sfake.NewClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("cannot create selfsubjectaccessreviews")
		})
	service := NewService(&stubRunner{}, client, "", "")

	support := service.Allowed(context.Background(), "monitoring")
	if !support.Allowed {
		t.Fatal("an advisory check must never be stricter than the real API")
	}
}

func TestWaitReportsTheLastWaitingReasonOnTimeout(t *testing.T) {
	pod := runningPod()
	client := k8sfake.NewClientset(pod)
	runner := &stubRunner{}
	runner.onRun = func() {
		current, _ := client.CoreV1().Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		current.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{{
			Name:  "spinoza-debug-1",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
		}}
		_, _ = client.CoreV1().Pods(pod.Namespace).UpdateStatus(context.Background(), current, metav1.UpdateOptions{})
	}
	service := NewService(runner, client, "", "")
	service.poll = time.Millisecond
	service.timeout = 40 * time.Millisecond

	_, err := service.Ensure(context.Background(), request())
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if !strings.Contains(err.Error(), "ContainerCreating") {
		t.Fatalf("message = %q, want the last waiting reason", err.Error())
	}
}
