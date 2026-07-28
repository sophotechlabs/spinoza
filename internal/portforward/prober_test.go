package portforward

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestProberSeesALivePod(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "flux-system"}}

	if !NewProber(k8sfake.NewClientset(pod)).Alive(context.Background(), "flux-system", "web") {
		t.Fatalf("an existing pod must report alive")
	}
}

func TestProberReportsAMissingPod(t *testing.T) {
	if NewProber(k8sfake.NewClientset()).Alive(context.Background(), "flux-system", "web") {
		t.Fatalf("a deleted pod must report dead")
	}
}

func TestProberKeepsTheForwardOnATransientError(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("get", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver unreachable")
	})

	if !NewProber(cs).Alive(context.Background(), "flux-system", "web") {
		t.Fatalf("a transient api error must not kill a working forward")
	}
}
