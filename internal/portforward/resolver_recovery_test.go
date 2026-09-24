package portforward

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestServiceResolutionCannotEscapeItsNamespaceOrSelector(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	foreign := readyPod("foreign", selector, nil)
	foreign.Namespace = "another-team"
	unrelated := readyPod("unrelated", map[string]string{"app": "other"}, nil)
	client := k8sfake.NewClientset(service(9090, intstr.FromString("web"), selector), foreign, unrelated)
	resolver := NewResolver(client)
	pod, port, err := resolver.Resolve(t.Context(), serviceTarget(), 9090)
	if err == nil || err.Error() != "no ready pod backs the service in monitoring" {
		t.Fatalf("resolve = %s:%d, %v, want refusal despite ready unrelated pods", pod, port, err)
	}
	if pod != "" || port != 0 {
		t.Fatalf("refused resolution = %s:%d, want no backend", pod, port)
	}
	eligible := readyPod("owned", selector, []corev1.ContainerPort{{Name: "web", ContainerPort: 9099}})
	if _, createErr := client.CoreV1().Pods("monitoring").Create(t.Context(), eligible, metav1.CreateOptions{}); createErr != nil {
		t.Fatal(createErr)
	}
	pod, port, err = resolver.Resolve(t.Context(), serviceTarget(), 9090)
	if err != nil || pod != "owned" || port != 9099 {
		t.Fatalf("recovered resolution = %s:%d, %v, want owned:9099", pod, port, err)
	}
}

func TestServiceResolutionUsesTheReplacementPodsReadinessAndNamedPort(t *testing.T) {
	selector := map[string]string{"app": "prom"}
	original := readyPod("original", selector, []corev1.ContainerPort{{Name: "web", ContainerPort: 9099}})
	client := k8sfake.NewClientset(service(9090, intstr.FromString("web"), selector), original)
	resolver := NewResolver(client)
	pod, port, err := resolver.Resolve(t.Context(), serviceTarget(), 9090)
	if err != nil || pod != "original" || port != 9099 {
		t.Fatalf("initial resolution = %s:%d, %v, want original:9099", pod, port, err)
	}
	if deleteErr := client.CoreV1().Pods("monitoring").Delete(t.Context(), "original", metav1.DeleteOptions{}); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	replacement := readyPod("replacement", selector, []corev1.ContainerPort{{Name: "web", ContainerPort: 9199}})
	replacement.Status.Conditions[0].Status = corev1.ConditionFalse
	if _, createErr := client.CoreV1().Pods("monitoring").Create(t.Context(), replacement, metav1.CreateOptions{}); createErr != nil {
		t.Fatal(createErr)
	}
	pod, port, err = resolver.Resolve(t.Context(), serviceTarget(), 9090)
	if err == nil || err.Error() != "no ready pod backs the service in monitoring" || pod != "" || port != 0 {
		t.Fatalf("unready replacement = %s:%d, %v, want refusal without reusing the deleted pod", pod, port, err)
	}
	replacement.Status.Conditions[0].Status = corev1.ConditionTrue
	if _, updateErr := client.CoreV1().Pods("monitoring").UpdateStatus(t.Context(), replacement, metav1.UpdateOptions{}); updateErr != nil {
		t.Fatal(updateErr)
	}
	pod, port, err = resolver.Resolve(t.Context(), serviceTarget(), 9090)
	if err != nil || pod != "replacement" || port != 9199 {
		t.Fatalf("recovered resolution = %s:%d, %v, want replacement:9199", pod, port, err)
	}
}
