package debugcontainer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
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
	service := NewService(runner, client, "", api.ContextRef{Name: "p-mk1"}, access.New(client))
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

func TestRunningStatusWithoutItsContainerSpecIsIgnored(t *testing.T) {
	pod := runningPod()
	pod.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{
		{Name: "spinoza-debug-1", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
	}

	spec, image, found := runningDebugContainer(pod)

	if found || spec != nil || image != "" {
		t.Fatalf("running container = %+v %q %v, want none without a matching spec", spec, image, found)
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
	service := NewService(&stubRunner{}, k8sfake.NewClientset(), "", api.ContextRef{}, permsOn(k8sfake.NewClientset()))
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
	service := NewService(runner, client, "", api.ContextRef{}, access.New(client))
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
	service := NewService(runner, client, "", api.ContextRef{}, access.New(client))
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
	synctest.Test(t, func(t *testing.T) {
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
	})
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
	service := NewService(runner, client, "", api.ContextRef{}, access.New(client))
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

func permsOn(cs kubernetes.Interface) *access.Service {
	return access.New(cs)
}

func permitting(t *testing.T, cs kubernetes.Interface) *Service {
	t.Helper()
	return NewService(&stubRunner{}, cs, "", api.ContextRef{}, permsOn(cs))
}

func TestAllowedReportsAPermittedNamespace(t *testing.T) {
	service := permitting(t, allowingClient(t, true, ""))

	support := service.Allowed(context.Background(), "monitoring", "loki-0")
	if !support.Allowed {
		t.Fatal("expected the namespace to be allowed")
	}
	if support.Namespace != "monitoring" {
		t.Fatalf("namespace = %q", support.Namespace)
	}
}

func TestAllowedReportsARefusalWithItsReason(t *testing.T) {
	service := permitting(t, allowingClient(t, false, "no RBAC policy matched"))

	support := service.Allowed(context.Background(), "kube-system", "loki-0")
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
	service := NewService(&stubRunner{}, client, "", api.ContextRef{}, permsOn(client))

	support := service.Allowed(context.Background(), "monitoring", "loki-0")
	if !support.Allowed {
		t.Fatal("an advisory check must never be stricter than the real API")
	}
	if !strings.Contains(support.Reason, "cannot create selfsubjectaccessreviews") {
		t.Fatalf("reason = %q, want the check failure carried rather than dropped", support.Reason)
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
	service := NewService(runner, client, "", api.ContextRef{}, access.New(client))
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

func debugPodWith(name string, common corev1.EphemeralContainerCommon) *corev1.Pod {
	pod := runningPod()
	common.Name = name
	pod.Spec.EphemeralContainers = []corev1.EphemeralContainer{{EphemeralContainerCommon: common}}
	pod.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{
		{
			Name:  name,
			Image: common.Image,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		},
	}
	return pod
}

func TestReuseReportsTheImageTheContainerActuallyRuns(t *testing.T) {
	pod := debugPodWith("spinoza-debug-1", corev1.EphemeralContainerCommon{Image: "alpine:3.20"})
	service := newService(t, pod, &stubRunner{})

	session, err := service.Ensure(context.Background(), request())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	if session.Image != "alpine:3.20" {
		t.Fatalf("image = %q, want the running container's image rather than the configured one", session.Image)
	}
}

func TestReuseReportsTheProfileTheContainerActuallyHas(t *testing.T) {
	yes := true
	pod := debugPodWith("spinoza-debug-1", corev1.EphemeralContainerCommon{
		Image:           "busybox:1.37",
		SecurityContext: &corev1.SecurityContext{Privileged: &yes},
	})
	service := newService(t, pod, &stubRunner{})

	session, err := service.Ensure(context.Background(), request())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	if session.Profile != "sysadmin" {
		t.Fatalf("profile = %q, want sysadmin read back from the container", session.Profile)
	}
}

func TestReuseRefusesToPretendAnUnprivilegedContainerIsSysadmin(t *testing.T) {
	pod := debugPodWith("spinoza-debug-1", corev1.EphemeralContainerCommon{
		Image: "busybox:1.37",
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"SYS_PTRACE"}},
		},
	})
	runner := &stubRunner{}
	service := newService(t, pod, runner)
	req := request()
	req.Profile = "sysadmin"

	_, err := service.Ensure(context.Background(), req)

	if err == nil {
		t.Fatal("asking for sysadmin silently returned the unprivileged container")
	}
	if !strings.Contains(err.Error(), "not privileged") {
		t.Fatalf("err = %v, want it to say why", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("kubectl ran %d times on a refusal", len(runner.calls))
	}
}

func TestReuseReportsTheContainerItIsAttachedTo(t *testing.T) {
	pod := debugPodWith("spinoza-debug-1", corev1.EphemeralContainerCommon{Image: "busybox:1.37"})
	pod.Spec.EphemeralContainers[0].TargetContainerName = "loki"
	service := newService(t, pod, &stubRunner{})

	session, err := service.Ensure(context.Background(), request())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	if session.Target != "loki" {
		t.Fatalf("target = %q, want the container the debug container shares a namespace with", session.Target)
	}
}

func TestProfileOfLeavesAnUnknownShapeUnnamed(t *testing.T) {
	cases := map[string]*corev1.SecurityContext{
		"absent":  nil,
		"unknown": {ReadOnlyRootFilesystem: new(bool)},
	}
	for name, security := range cases {
		spec := &corev1.EphemeralContainer{
			EphemeralContainerCommon: corev1.EphemeralContainerCommon{SecurityContext: security},
		}
		if got := profileOf(spec); got != "" {
			t.Fatalf("%s: profileOf = %q, want an empty string rather than a guess", name, got)
		}
	}
}

func TestProfileOfNamesTheShapesKubectlWrites(t *testing.T) {
	yes := true
	cases := map[string]struct {
		security *corev1.SecurityContext
		want     string
	}{
		"sysadmin": {&corev1.SecurityContext{Privileged: &yes}, "sysadmin"},
		"netadmin": {&corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN", "NET_RAW"}},
		}, "netadmin"},
		"general": {&corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"SYS_PTRACE"}},
		}, "general"},
		"restricted": {&corev1.SecurityContext{RunAsNonRoot: &yes}, "restricted"},
	}
	for name, tc := range cases {
		spec := &corev1.EphemeralContainer{
			EphemeralContainerCommon: corev1.EphemeralContainerCommon{SecurityContext: tc.security},
		}
		if got := profileOf(spec); got != tc.want {
			t.Fatalf("%s: profileOf = %q, want %q", name, got, tc.want)
		}
	}
}

func TestAllowedAsksAboutTheSpecificPod(t *testing.T) {
	service := newService(t, runningPod(), &stubRunner{})
	clientset, ok := service.cs.(*k8sfake.Clientset)
	if !ok {
		t.Fatal("expected the fake clientset")
	}
	var seen *authv1.ResourceAttributes
	clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, isCreate := action.(k8stesting.CreateAction)
		if !isCreate {
			return false, nil, nil
		}
		review, isReview := create.GetObject().(*authv1.SelfSubjectAccessReview)
		if !isReview {
			return false, nil, nil
		}
		seen = review.Spec.ResourceAttributes
		review.Status.Allowed = true
		return true, review, nil
	})

	service.Allowed(context.Background(), "monitoring", "loki-0")

	if seen == nil {
		t.Fatal("no access review was sent")
	}
	if seen.Name != "loki-0" {
		t.Fatalf("review name = %q; a resourceNames-scoped Role would read as denied", seen.Name)
	}
}

func TestAllowedReportsTheImageItWouldUse(t *testing.T) {
	service := newService(t, runningPod(), &stubRunner{})

	support := service.Allowed(context.Background(), "monitoring", "loki-0")

	if support.Image == "" {
		t.Fatal("support did not report the image the prompt has to show")
	}
	if support.Pod != "loki-0" {
		t.Fatalf("pod = %q", support.Pod)
	}
}

type blockingRunner struct {
	saw chan error
}

func (b *blockingRunner) Run(ctx context.Context, _ []string) error {
	<-ctx.Done()
	b.saw <- ctx.Err()
	return ctx.Err()
}

func TestEnsureBoundsTheKubectlPatchPhase(t *testing.T) {
	previous := patchTimeout
	patchTimeout = 150 * time.Millisecond
	t.Cleanup(func() {
		patchTimeout = previous
	})
	runner := &blockingRunner{saw: make(chan error, 1)}
	service := NewService(runner, k8sfake.NewClientset(runningPod()), "", api.ContextRef{Name: "p-mk1"}, permsOn(k8sfake.NewClientset(runningPod())))

	started := time.Now()
	_, err := service.Ensure(context.Background(), request())
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("a kubectl that never returns produced no error")
	}
	if elapsed > time.Second {
		t.Fatalf("Ensure took %s, so it was not the cap that ended it", elapsed)
	}
	if !errors.Is(<-runner.saw, context.DeadlineExceeded) {
		t.Fatal("kubectl was ended by something other than the patch deadline")
	}
}

func TestEnsureLeavesTheKubectlDeadlineAloneWhenTheCallerIsShorter(t *testing.T) {
	runner := &blockingRunner{saw: make(chan error, 1)}
	service := NewService(runner, k8sfake.NewClientset(runningPod()), "", api.ContextRef{Name: "p-mk1"}, permsOn(k8sfake.NewClientset(runningPod())))
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := service.Ensure(ctx, request())

	if err == nil {
		t.Fatal("a caller whose deadline passed produced no error")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("Ensure outlived its caller by %s", time.Since(started))
	}
	if !errors.Is(<-runner.saw, context.DeadlineExceeded) {
		t.Fatal("kubectl did not see the caller's deadline")
	}
}

func TestTheKubectlArgsCarryTheContextAndKubeconfig(t *testing.T) {
	service := NewService(&stubRunner{}, k8sfake.NewClientset(runningPod()), "", api.ContextRef{
		Name:       "kind-spinoza",
		Kubeconfig: "/home/arch/.kube/config",
	}, permsOn(k8sfake.NewClientset(runningPod())))

	args := service.args(t.Context(), request(), "spinoza-debug-1", "general")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--context kind-spinoza") {
		t.Fatalf("args = %v, want the context named", args)
	}
	if !strings.Contains(joined, "--kubeconfig /home/arch/.kube/config") {
		t.Fatalf("args = %v, want the kubeconfig named", args)
	}
}

func TestAContainerThatIsNeitherRunningNorWaitingIsStillSettling(t *testing.T) {
	pod := runningPod()
	pod.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{{Name: "spinoza-debug-1"}}

	running, waiting, err := progressOf(pod, "spinoza-debug-1")

	if running || waiting != "" || err != nil {
		t.Fatalf("progress = %v %q %v, want it read as still settling", running, waiting, err)
	}
}

func TestAContainerSpecIsOnlyFoundByItsOwnName(t *testing.T) {
	pod := runningPod()
	pod.Spec.EphemeralContainers = []corev1.EphemeralContainer{{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "spinoza-debug-1"},
	}}

	if specFor(pod, "spinoza-debug-1") == nil {
		t.Fatal("the container it does have was not found")
	}
	if specFor(pod, "spinoza-debug-2") != nil {
		t.Fatal("a container it does not have was found")
	}
}

func TestPrivilegeIsOnlyClaimedWhenItIsAskedFor(t *testing.T) {
	bare := &corev1.EphemeralContainer{}
	if privileged(bare) {
		t.Fatal("a container with no security context read as privileged")
	}
	empty := &corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{SecurityContext: &corev1.SecurityContext{}},
	}
	if privileged(empty) {
		t.Fatal("a container that never set privileged read as privileged")
	}
	yes := true
	asked := &corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			SecurityContext: &corev1.SecurityContext{Privileged: &yes},
		},
	}
	if !privileged(asked) {
		t.Fatal("a privileged container did not read as privileged")
	}
}

func TestAPodNameThatIsNotOneIsRefused(t *testing.T) {
	err := admits(runningPod(), Request{Namespace: "monitoring", Pod: "Loki_0"})

	if err == nil {
		t.Fatal("admits accepted a pod name kubernetes would not")
	}
}

func TestARefusalWithoutAReasonStillSaysWhat(t *testing.T) {
	service := permitting(t, allowingClient(t, false, ""))

	support := service.Allowed(context.Background(), "kube-system", "loki-0")

	if support.Allowed {
		t.Fatal("a refusal was reported as permission")
	}
	if !strings.Contains(support.Reason, "ephemeral container") {
		t.Fatalf("reason = %q, want a sentence when the cluster gave none", support.Reason)
	}
}

func TestTheAnswerIsRememberedBetweenPrompts(t *testing.T) {
	asked := 0
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		asked++
		return true, &authv1.SelfSubjectAccessReview{
			Status: authv1.SubjectAccessReviewStatus{Allowed: true},
		}, nil
	})
	service := permitting(t, cs)

	service.Allowed(context.Background(), "monitoring", "loki-0")
	service.Allowed(context.Background(), "monitoring", "loki-0")

	if asked != 1 {
		t.Fatalf("asked the cluster %d times, want the second answered from memory", asked)
	}
}

func TestAnotherPodIsItsOwnQuestion(t *testing.T) {
	asked := 0
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		asked++
		return true, &authv1.SelfSubjectAccessReview{
			Status: authv1.SubjectAccessReviewStatus{Allowed: true},
		}, nil
	})
	service := permitting(t, cs)

	service.Allowed(context.Background(), "monitoring", "loki-0")
	service.Allowed(context.Background(), "monitoring", "loki-1")

	if asked != 2 {
		t.Fatalf("asked the cluster %d times, want each pod asked about", asked)
	}
}

func TestAQuestionThatCouldNotBePutLeavesTheButtonAlone(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("the apiserver would not answer")
	})
	service := permitting(t, cs)

	support := service.Allowed(context.Background(), "monitoring", "loki-0")

	if !support.Allowed {
		t.Fatal("a question that could not be put took the button away")
	}
	if !strings.Contains(support.Reason, "could not check") {
		t.Fatalf("reason = %q, want it clear that nothing was found out", support.Reason)
	}
}
