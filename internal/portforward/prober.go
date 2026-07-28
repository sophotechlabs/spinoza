package portforward

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type podProber struct {
	cs kubernetes.Interface
}

func NewProber(cs kubernetes.Interface) Prober {
	return &podProber{cs: cs}
}

func (p *podProber) Alive(ctx context.Context, namespace, pod string) bool {
	_, err := p.cs.CoreV1().Pods(namespace).Get(ctx, pod, metav1.GetOptions{})
	if err == nil {
		return true
	}
	return !apierrors.IsNotFound(err)
}
