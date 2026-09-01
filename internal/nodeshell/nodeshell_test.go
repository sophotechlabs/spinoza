package nodeshell

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/access"
)

func running(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: DefaultNamespace},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func service(t *testing.T, enabled bool, objs ...runtime.Object) (*Service, *k8sfake.Clientset) {
	t.Helper()
	cs := k8sfake.NewClientset(objs...)
	return NewService(cs, "busybox:1.37", "", func() bool { return enabled }, access.New(cs)), cs
}

func creates(cs *k8sfake.Clientset, name string, phase corev1.PodPhase, reason string) {
	cs.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		pod, ok := create.GetObject().(*corev1.Pod)
		if !ok {
			return false, nil, nil
		}
		pod.Name = name
		pod.Status.Phase = phase
		pod.Status.Reason = reason
		return false, pod, nil
	})
}

func allow(cs *k8sfake.Clientset, allowed bool) {
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.SelfSubjectAccessReview{Status: authv1.SubjectAccessReviewStatus{Allowed: allowed}}, nil
	})
}

func TestItStaysOffUntilItIsTurnedOn(t *testing.T) {
	svc, _ := service(t, false)

	support := svc.Support(t.Context(), "p-mk1")

	if support.Enabled || support.Allowed {
		t.Fatalf("support = %+v, want it off", support)
	}
	if support.Reason == "" {
		t.Fatal("an off node shell said nothing about how to turn it on")
	}
}

func TestTurningItOnIsSeenWithoutARestart(t *testing.T) {
	cs := k8sfake.NewClientset()
	allow(cs, true)
	on := false
	svc := NewService(cs, "busybox:1.37", "", func() bool { return on }, access.New(cs))

	before := svc.Support(t.Context(), "p-mk1")
	on = true
	after := svc.Support(t.Context(), "p-mk1")

	if before.Enabled {
		t.Fatal("it was enabled before anything turned it on")
	}
	if !after.Enabled || !after.Allowed {
		t.Fatalf("support = %+v, want it on and allowed", after)
	}
}

func TestNoWayToAskLeavesItOff(t *testing.T) {
	svc := NewService(k8sfake.NewClientset(), "busybox:1.37", "", nil, nil)

	support := svc.Support(t.Context(), "p-mk1")

	if support.Enabled {
		t.Fatal("a service with nothing to ask reported itself on")
	}
}

func TestItReportsTheImageAndNamespaceItWouldUse(t *testing.T) {
	svc, cs := service(t, true)
	allow(cs, true)

	support := svc.Support(t.Context(), "p-mk1")

	if support.Image != "busybox:1.37" || support.Namespace != DefaultNamespace {
		t.Fatalf("support = %+v, want the image and namespace named", support)
	}
	if !support.Allowed {
		t.Fatalf("support = %+v, want it allowed", support)
	}
}

func TestItSaysWhenYouMayNotCreatePods(t *testing.T) {
	svc, cs := service(t, true)
	allow(cs, false)

	support := svc.Support(t.Context(), "p-mk1")

	if support.Allowed {
		t.Fatal("a refused access review was reported as allowed")
	}
	if support.Reason == "" {
		t.Fatal("a refusal came back without a reason")
	}
}

func TestItNeedsANodeToCheckAgainst(t *testing.T) {
	svc, cs := service(t, true)
	allow(cs, true)

	if svc.Support(t.Context(), "").Reason != "no node was named" {
		t.Fatal("an empty node was not refused")
	}
}

func TestAReviewThatFailsIsReportedNotSwallowed(t *testing.T) {
	svc, cs := service(t, true)
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("the apiserver said no")
	})

	support := svc.Support(t.Context(), "p-mk1")

	if support.Allowed {
		t.Fatal("a failed review was treated as permission")
	}
	if support.Reason == "" {
		t.Fatal("a failed review came back without a reason")
	}
}

func TestThePodJoinsTheHostNamespaces(t *testing.T) {
	svc, _ := service(t, true)

	pod := svc.pod("p-mk1")

	if !pod.Spec.HostPID || !pod.Spec.HostIPC || !pod.Spec.HostNetwork {
		t.Fatalf("spec = %+v, want the host namespaces joined", pod.Spec)
	}
	if pod.Spec.NodeName != "p-mk1" {
		t.Fatalf("nodeName = %q, want the node it was asked for", pod.Spec.NodeName)
	}
	if pod.Spec.Containers[0].SecurityContext.Privileged == nil || !*pod.Spec.Containers[0].SecurityContext.Privileged {
		t.Fatal("the container is not privileged, so nsenter could not enter the host")
	}
}

func TestThePodToleratesEveryTaint(t *testing.T) {
	svc, _ := service(t, true)

	pod := svc.pod("p-mk1")

	if len(pod.Spec.Tolerations) != 1 || pod.Spec.Tolerations[0].Operator != corev1.TolerationOpExists {
		t.Fatalf("tolerations = %+v, want one that matches every taint", pod.Spec.Tolerations)
	}
}

func TestThePodSaysWhoMadeItAndWhy(t *testing.T) {
	svc, _ := service(t, true)

	pod := svc.pod("p-mk1")

	if pod.Labels[managedBy] != owner || pod.Labels[nodeLabel] != "p-mk1" {
		t.Fatalf("labels = %v, want spinoza and the node named", pod.Labels)
	}
	if pod.GenerateName == "" {
		t.Fatal("the pod has a fixed name, so two shells would collide")
	}
}

func TestThePodOnlySleeps(t *testing.T) {
	svc, _ := service(t, true)

	pod := svc.pod("p-mk1")

	if pod.Spec.Containers[0].Command[0] != "sleep" {
		t.Fatalf("command = %v, want it to wait while the shell execs in", pod.Spec.Containers[0].Command)
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatalf("restart policy = %q, want it not to come back", pod.Spec.RestartPolicy)
	}
}

func TestTheShellEntersTheHostNamespaces(t *testing.T) {
	if Enter[0] != "nsenter" {
		t.Fatalf("command = %v, want nsenter", Enter)
	}
	for _, flag := range []string{"--mount", "--uts", "--ipc", "--net", "--pid"} {
		if !contains(Enter, flag) {
			t.Fatalf("command = %v, want %s", Enter, flag)
		}
	}
}

func contains(list []string, want string) bool {
	return slices.Contains(list, want)
}

func TestStartWaitsUntilThePodRuns(t *testing.T) {
	svc, cs := service(t, true)
	creates(cs, "spinoza-node-shell-abc", corev1.PodRunning, "")

	session, err := svc.Start(t.Context(), "p-mk1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if session.Pod != "spinoza-node-shell-abc" || session.Container != container {
		t.Fatalf("session = %+v", session)
	}
	if session.Node != "p-mk1" || session.Namespace != DefaultNamespace {
		t.Fatalf("session = %+v, want the node and namespace named", session)
	}
}

func TestStartRefusesWhileOff(t *testing.T) {
	svc, _ := service(t, false)

	_, err := svc.Start(t.Context(), "p-mk1")

	if err == nil {
		t.Fatal("a node shell started while the feature was off")
	}
}

func TestStartNeedsANode(t *testing.T) {
	svc, _ := service(t, true)

	_, err := svc.Start(t.Context(), "")

	if err == nil {
		t.Fatal("a node shell started without a node")
	}
}

func TestAPodThatFailsIsReportedAndRemoved(t *testing.T) {
	svc, cs := service(t, true)
	creates(cs, "spinoza-node-shell-bad", corev1.PodFailed, "OutOfpods")
	deleted := false
	cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		deleted = true
		return true, nil, nil
	})

	_, err := svc.Start(t.Context(), "p-mk1")

	if err == nil {
		t.Fatal("a failed pod was reported as a working shell")
	}
	if !deleted {
		t.Fatal("a failed pod was left behind")
	}
}

func TestAStartFailureIncludesACleanupFailure(t *testing.T) {
	svc, cs := service(t, true)
	creates(cs, "spinoza-node-shell-bad", corev1.PodFailed, "ImagePullBackOff")
	cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: "pods"},
			"spinoza-node-shell-bad",
			errors.New("cleanup was refused"),
		)
	})

	_, err := svc.Start(t.Context(), "p-mk1")

	if err == nil {
		t.Fatal("a failed start and cleanup reported success")
	}
	if !strings.Contains(err.Error(), "ImagePullBackOff") {
		t.Fatalf("error = %v, want the pod failure", err)
	}
	if !strings.Contains(err.Error(), "cleanup was refused") {
		t.Fatalf("error = %v, want the cleanup failure", err)
	}
}

func TestAPodThatNeverStartsGivesUp(t *testing.T) {
	svc, cs := service(t, true)
	creates(cs, "spinoza-node-shell-slow", corev1.PodPending, "")
	was := startTimeout
	startTimeout = 10 * time.Millisecond
	pollWas := pollEvery
	pollEvery = time.Millisecond
	t.Cleanup(func() {
		startTimeout = was
		pollEvery = pollWas
	})

	_, err := svc.Start(t.Context(), "p-mk1")

	if err == nil {
		t.Fatal("a pod that never ran was reported as a working shell")
	}
}

func TestAStartWhosePodCannotBeReadIsReportedAndRemoved(t *testing.T) {
	svc, cs := service(t, true)
	creates(cs, "spinoza-node-shell-unreadable", corev1.PodPending, "")
	cs.PrependReactor("get", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: "pods"},
			"spinoza-node-shell-unreadable",
			errors.New("pod reads are forbidden"),
		)
	})
	deleted := false
	cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		deleted = true
		return false, nil, nil
	})

	_, err := svc.Start(t.Context(), "p-mk1")

	if err == nil || !strings.Contains(err.Error(), "pod reads are forbidden") {
		t.Fatalf("start error = %v, want the pod read failure", err)
	}
	if !deleted {
		t.Fatal("an unreadable node shell pod was left behind")
	}
}

func TestCancelingAStartStillRemovesThePod(t *testing.T) {
	svc, cs := service(t, true)
	creates(cs, "spinoza-node-shell-canceled", corev1.PodPending, "")
	deleted := false
	cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		deleted = true
		return false, nil, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := svc.Start(ctx, "p-mk1")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("start error = %v, want cancellation", err)
	}
	if !deleted {
		t.Fatal("a canceled node shell pod was left behind")
	}
}

func TestRemoveDeletesThePod(t *testing.T) {
	svc, cs := service(t, true, running("spinoza-node-shell-abc"))

	if err := svc.Remove(t.Context(), "spinoza-node-shell-abc"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err := cs.CoreV1().Pods(DefaultNamespace).Get(t.Context(), "spinoza-node-shell-abc", metav1.GetOptions{})
	if err == nil {
		t.Fatal("the pod outlived the shell")
	}
}

func TestRemoveTreatsAMissingPodAsAlreadyGone(t *testing.T) {
	svc, _ := service(t, true)

	if err := svc.Remove(t.Context(), "spinoza-node-shell-gone"); err != nil {
		t.Fatalf("remove missing pod: %v", err)
	}
}

func TestRemoveWithNoPodDoesNothing(t *testing.T) {
	svc, cs := service(t, true)
	called := false
	cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		called = true
		return true, nil, nil
	})

	if err := svc.Remove(t.Context(), ""); err != nil {
		t.Fatalf("remove empty name: %v", err)
	}

	if called {
		t.Fatal("an empty pod name still reached the apiserver")
	}
}

func TestRemoveRetriesATransientFailure(t *testing.T) {
	svc, cs := service(t, true, running("spinoza-node-shell-abc"))
	svc.removeEvery = time.Millisecond
	deletes := 0
	cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		deletes++
		if deletes == 1 {
			return true, nil, errors.New("the connection was reset")
		}
		return false, nil, nil
	})

	if err := svc.Remove(t.Context(), "spinoza-node-shell-abc"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if deletes != 2 {
		t.Fatalf("delete attempts = %d, want one retry", deletes)
	}
}

func TestRemoveReportsAnErrorThatOutlivesItsContext(t *testing.T) {
	svc, cs := service(t, true, running("spinoza-node-shell-abc"))
	svc.removeEvery = time.Millisecond
	cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("the connection was reset")
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	err := svc.Remove(ctx, "spinoza-node-shell-abc")

	if err == nil || !strings.Contains(err.Error(), "the connection was reset") {
		t.Fatalf("remove error = %v, want the apiserver failure", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("remove error = %v, want the cleanup deadline", err)
	}
}

func TestPermanentDeleteErrorsAreNotRetried(t *testing.T) {
	reasons := []metav1.StatusReason{
		metav1.StatusReasonBadRequest,
		metav1.StatusReasonForbidden,
		metav1.StatusReasonInvalid,
		metav1.StatusReasonMethodNotAllowed,
		metav1.StatusReasonUnauthorized,
	}
	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			svc, cs := service(t, true, running("spinoza-node-shell-abc"))
			attempts := 0
			cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
				attempts++
				return true, nil, &apierrors.StatusError{ErrStatus: metav1.Status{Reason: reason}}
			})

			err := svc.Remove(t.Context(), "spinoza-node-shell-abc")

			if err == nil {
				t.Fatal("a permanent deletion failure reported success")
			}
			if attempts != 1 {
				t.Fatalf("delete attempts = %d, want no retry", attempts)
			}
		})
	}
}

func TestThePodCannotOutliveTheDay(t *testing.T) {
	svc, _ := service(t, true)

	pod := svc.pod("p-mk1")

	if pod.Spec.ActiveDeadlineSeconds == nil {
		t.Fatal("a privileged pod was created with no deadline, so an abandoned one would run forever")
	}
	if *pod.Spec.ActiveDeadlineSeconds > int64((4 * time.Hour).Seconds()) {
		t.Fatalf("deadline = %ds, want it short enough to matter", *pod.Spec.ActiveDeadlineSeconds)
	}
}

func TestItRepeatsWhyTheApiserverSaidNo(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.SelfSubjectAccessReview{Status: authv1.SubjectAccessReviewStatus{
			Allowed: false,
			Reason:  `no RBAC policy matched for user "arch"`,
		}}, nil
	})
	svc := NewService(cs, "busybox:1.37", "", func() bool { return true }, access.New(cs))

	support := svc.Support(t.Context(), "p-mk1")

	if support.Allowed {
		t.Fatal("a refusal was read as permission")
	}
	if support.Reason != `no RBAC policy matched for user "arch"` {
		t.Fatalf("reason = %q, want the apiserver's own words", support.Reason)
	}
}

func TestAPodTheClusterRefusesIsReported(t *testing.T) {
	cs := k8sfake.NewClientset()
	allow(cs, true)
	cs.PrependReactor("create", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: "pods"}, "",
			errors.New("privileged pods are not allowed in this namespace"),
		)
	})
	svc := NewService(cs, "busybox:1.37", "", func() bool { return true }, access.New(cs))

	_, err := svc.Start(t.Context(), "p-mk1")

	if err == nil {
		t.Fatal("a refused pod reported a working shell")
	}
	if !strings.Contains(err.Error(), "privileged pods are not allowed") {
		t.Fatalf("error = %v, want what the cluster said", err)
	}
}

func TestAPodThatNeverStartsIsTakenAwayAgain(t *testing.T) {
	restore := hurry(t)
	defer restore()
	cs := k8sfake.NewClientset()
	allow(cs, true)
	creates(cs, "spinoza-node-shell-slow", corev1.PodPending, "")
	svc := NewService(cs, "busybox:1.37", "", func() bool { return true }, access.New(cs))

	_, err := svc.Start(t.Context(), "p-mk1")

	if err == nil {
		t.Fatal("a pod that never ran reported a working shell")
	}
	if !strings.Contains(err.Error(), "did not start within") {
		t.Fatalf("error = %v, want the wait to be named", err)
	}
	left, listErr := cs.CoreV1().Pods(DefaultNamespace).List(t.Context(), metav1.ListOptions{})
	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}
	if len(left.Items) != 0 {
		t.Fatalf("pods = %d, want the one that never started taken away", len(left.Items))
	}
}

func TestACancelledStartStillRetriesPodCleanup(t *testing.T) {
	cs := k8sfake.NewClientset()
	allow(cs, true)
	creates(cs, "spinoza-node-shell-slow", corev1.PodPending, "")
	svc := NewService(cs, "busybox:1.37", "", func() bool { return true }, access.New(cs))
	svc.removeEvery = time.Millisecond
	deletes := 0
	cs.PrependReactor("delete", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		deletes++
		if deletes == 1 {
			return true, nil, errors.New("the connection was reset")
		}
		return false, nil, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := svc.Start(ctx, "p-mk1")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("start error = %v, want cancellation", err)
	}
	if deletes != 2 {
		t.Fatalf("delete attempts = %d, want cleanup to outlive cancellation and retry", deletes)
	}
	left, listErr := cs.CoreV1().Pods(DefaultNamespace).List(t.Context(), metav1.ListOptions{})
	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}
	if len(left.Items) != 0 {
		t.Fatalf("pods = %d, want the canceled shell taken away", len(left.Items))
	}
}

func TestAPodThatCannotBeReadWhileWaitingIsReported(t *testing.T) {
	restore := hurry(t)
	defer restore()
	cs := k8sfake.NewClientset()
	allow(cs, true)
	creates(cs, "spinoza-node-shell-gone", corev1.PodPending, "")
	cs.PrependReactor("get", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("the connection was reset")
	})
	svc := NewService(cs, "busybox:1.37", "", func() bool { return true }, access.New(cs))

	_, err := svc.Start(t.Context(), "p-mk1")

	if err == nil || !strings.Contains(err.Error(), "connection was reset") {
		t.Fatalf("error = %v, want the read failure", err)
	}
}

func hurry(t *testing.T) func() {
	t.Helper()
	oldTimeout, oldPoll := startTimeout, pollEvery
	startTimeout = 60 * time.Millisecond
	pollEvery = 10 * time.Millisecond
	return func() {
		startTimeout, pollEvery = oldTimeout, oldPoll
	}
}

func TestItRemembersWhatTheClusterSaid(t *testing.T) {
	cs := k8sfake.NewClientset()
	asked := 0
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		asked++
		return true, &authv1.SelfSubjectAccessReview{
			Status: authv1.SubjectAccessReviewStatus{Allowed: true},
		}, nil
	})
	svc := NewService(cs, "busybox:1.37", "", func() bool { return true }, access.New(cs))

	svc.Support(t.Context(), "p-mk1")
	svc.Support(t.Context(), "p-mk1")

	if asked != 1 {
		t.Fatalf("asked the cluster %d times, want the second answered from memory", asked)
	}
}

func TestAQuestionThatCouldNotBePutLeavesItUnoffered(t *testing.T) {
	svc, cs := service(t, true)
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.SelfSubjectAccessReview{
			Status: authv1.SubjectAccessReviewStatus{
				Allowed:         false,
				EvaluationError: "the webhook authorizer did not answer",
			},
		}, nil
	})

	support := svc.Support(t.Context(), "p-mk1")

	if support.Allowed {
		t.Fatal("an authorizer that could not decide was treated as permission")
	}
	if !strings.Contains(support.Reason, "could not check") {
		t.Fatalf("reason = %q, want it clear that nothing was found out", support.Reason)
	}
}

func TestAServiceWithNothingToAskWithSaysSo(t *testing.T) {
	svc := NewService(k8sfake.NewClientset(), "busybox:1.37", "", func() bool { return true }, nil)

	support := svc.Support(t.Context(), "p-mk1")

	if support.Allowed {
		t.Fatal("a service that asked nobody offered the shell anyway")
	}
	if !strings.Contains(support.Reason, "could not check") {
		t.Fatalf("reason = %q, want it clear that nothing was asked", support.Reason)
	}
}
