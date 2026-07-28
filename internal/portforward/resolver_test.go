package portforward

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func service(port int32, target intstr.IntOrString, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "prometheus", Namespace: "monitoring"},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports:    []corev1.ServicePort{{Name: "http", Port: port, TargetPort: target}},
		},
	}
}

func readyPod(name string, labels map[string]string, ports []corev1.ContainerPort) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "monitoring", Labels: labels},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Ports: ports}},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
}

func serviceTarget() Target {
	return Target{Kind: KindService, Namespace: "monitoring", Name: "prometheus"}
}

func TestResolvePodPassesThrough(t *testing.T) {
	resolver := NewResolver(k8sfake.NewClientset())

	pod, port, err := resolver.Resolve(context.Background(), podTarget(), 8080)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pod != "web" || port != 8080 {
		t.Fatalf("resolved %s:%d, want web:8080", pod, port)
	}
}

func TestResolveServiceWithANumericTargetPort(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	cs := k8sfake.NewClientset(
		service(9090, intstr.FromInt32(9091), selector),
		readyPod("prom-0", selector, nil),
	)

	pod, port, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pod != "prom-0" {
		t.Fatalf("pod = %q", pod)
	}
	if port != 9091 {
		t.Fatalf("port = %d, want the numeric targetPort 9091", port)
	}
}

func TestResolveServiceWithANamedTargetPort(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	cs := k8sfake.NewClientset(
		service(9090, intstr.FromString("web"), selector),
		readyPod("prom-0", selector, []corev1.ContainerPort{{Name: "web", ContainerPort: 9099}}),
	)

	_, port, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if port != 9099 {
		t.Fatalf("port = %d, want the named port resolved to 9099", port)
	}
}

func TestResolveServiceFallsBackToTheServicePort(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	cs := k8sfake.NewClientset(
		service(9090, intstr.IntOrString{}, selector),
		readyPod("prom-0", selector, nil),
	)

	_, port, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if port != 9090 {
		t.Fatalf("port = %d, want the service port when no targetPort is set", port)
	}
}

func TestResolveServiceRejectsAnUnknownPort(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	cs := k8sfake.NewClientset(
		service(9090, intstr.FromInt32(9090), selector),
		readyPod("prom-0", selector, nil),
	)

	_, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 1234)

	if err == nil {
		t.Fatalf("expected an error for a port the service does not expose")
	}
}

func TestResolveServiceWithoutASelector(t *testing.T) {
	cs := k8sfake.NewClientset(service(9090, intstr.FromInt32(9090), nil))

	_, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)

	if err == nil {
		t.Fatalf("expected an error for a selectorless service")
	}
}

func TestResolveServiceWithNoReadyPod(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	pending := readyPod("prom-0", selector, nil)
	pending.Status.Phase = corev1.PodPending
	cs := k8sfake.NewClientset(service(9090, intstr.FromInt32(9090), selector), pending)

	_, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)

	if err == nil {
		t.Fatalf("expected an error when no pod is running")
	}
}

func TestResolveServiceSkipsAnUnreadyPod(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	unready := readyPod("prom-0", selector, nil)
	unready.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
	ready := readyPod("prom-1", selector, nil)
	cs := k8sfake.NewClientset(service(9090, intstr.FromInt32(9090), selector), unready, ready)

	pod, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pod != "prom-1" {
		t.Fatalf("pod = %q, want the ready one", pod)
	}
}

func TestResolveServiceIgnoresUnrelatedConditions(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	pod := readyPod("prom-0", selector, nil)
	pod.Status.Conditions = append(
		[]corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}},
		pod.Status.Conditions...,
	)
	cs := k8sfake.NewClientset(service(9090, intstr.FromInt32(9090), selector), pod)

	got, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "prom-0" {
		t.Fatalf("pod = %q", got)
	}
}

func TestResolveServiceSkipsAPodWithoutAReadyCondition(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	bare := readyPod("prom-0", selector, nil)
	bare.Status.Conditions = nil
	cs := k8sfake.NewClientset(service(9090, intstr.FromInt32(9090), selector), bare)

	_, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)

	if err == nil {
		t.Fatalf("expected an error when no pod reports readiness")
	}
}

func TestResolveServiceRejectsAMissingNamedPort(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	cs := k8sfake.NewClientset(
		service(9090, intstr.FromString("missing"), selector),
		readyPod("prom-0", selector, []corev1.ContainerPort{{Name: "web", ContainerPort: 9099}}),
	)

	_, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)

	if err == nil {
		t.Fatalf("expected an error for a named port the pod does not declare")
	}
}

func TestResolveServiceSurfacesAGetFailure(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("get", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("boom")
	})

	_, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)

	if err == nil {
		t.Fatalf("expected the get failure to surface")
	}
}

func TestResolveServiceSurfacesAListFailure(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	cs := k8sfake.NewClientset(service(9090, intstr.FromInt32(9090), selector))
	cs.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("boom")
	})

	_, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)

	if err == nil {
		t.Fatalf("expected the list failure to surface")
	}
}

func TestResolveRejectsAnUnsupportedKind(t *testing.T) {
	target := Target{Kind: "Deployment", Namespace: "apps", Name: "web"}

	_, _, err := NewResolver(k8sfake.NewClientset()).Resolve(context.Background(), target, 8080)

	if err == nil {
		t.Fatalf("expected an error for an unsupported kind")
	}
}

func TestResolveServiceRejectsAnOutOfRangeTargetPort(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	cs := k8sfake.NewClientset(
		service(9090, intstr.FromInt(70000), selector),
		readyPod("prom-0", selector, nil),
	)

	_, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)

	if err == nil {
		t.Fatalf("expected an error for a target port above 65535")
	}
}

func TestResolveServiceRejectsANegativeTargetPort(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	cs := k8sfake.NewClientset(
		service(9090, intstr.FromInt(-1), selector),
		readyPod("prom-0", selector, nil),
	)

	_, _, err := NewResolver(cs).Resolve(context.Background(), serviceTarget(), 9090)

	if err == nil {
		t.Fatalf("expected an error for a negative target port")
	}
}
