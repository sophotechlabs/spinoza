package portforward

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

type podResolver struct {
	cs kubernetes.Interface
}

func NewResolver(cs kubernetes.Interface) Resolver {
	return &podResolver{cs: cs}
}

func (p *podResolver) Resolve(ctx context.Context, target Target, port int32) (string, int32, error) {
	if target.Kind == KindPod {
		return target.Name, port, nil
	}
	if target.Kind == KindService {
		return p.resolveService(ctx, target, port)
	}
	return "", 0, fmt.Errorf("cannot port forward to a %s", target.Kind)
}

func (p *podResolver) resolveService(ctx context.Context, target Target, port int32) (string, int32, error) {
	service, err := p.cs.CoreV1().Services(target.Namespace).Get(ctx, target.Name, metav1.GetOptions{})
	if err != nil {
		return "", 0, err
	}
	servicePort, found := portByNumber(service, port)
	if !found {
		return "", 0, fmt.Errorf("service %s/%s does not expose port %d", target.Namespace, target.Name, port)
	}
	if len(service.Spec.Selector) == 0 {
		return "", 0, fmt.Errorf("service %s/%s has no selector", target.Namespace, target.Name)
	}

	pod, err := p.readyPod(ctx, target.Namespace, service.Spec.Selector)
	if err != nil {
		return "", 0, err
	}
	podPort, err := targetPort(pod, servicePort)
	if err != nil {
		return "", 0, err
	}
	return pod.Name, podPort, nil
}

func portByNumber(service *corev1.Service, port int32) (corev1.ServicePort, bool) {
	for _, candidate := range service.Spec.Ports {
		if candidate.Port == port {
			return candidate, true
		}
	}
	return corev1.ServicePort{}, false
}

func (p *podResolver) readyPod(ctx context.Context, namespace string, selector map[string]string) (*corev1.Pod, error) {
	list, err := p.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(selector).String(),
	})
	if err != nil {
		return nil, err
	}
	for i := range list.Items {
		pod := &list.Items[i]
		if isReady(pod) {
			return pod, nil
		}
	}
	return nil, fmt.Errorf("no ready pod backs the service in %s", namespace)
}

func isReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type != corev1.PodReady {
			continue
		}
		return condition.Status == corev1.ConditionTrue
	}
	return false
}

const maxPort = 65535

func targetPort(pod *corev1.Pod, servicePort corev1.ServicePort) (int32, error) {
	numeric := servicePort.TargetPort.IntValue()
	if numeric != 0 {
		if numeric < 1 {
			return 0, fmt.Errorf("target port %d is out of range", numeric)
		}
		if numeric > maxPort {
			return 0, fmt.Errorf("target port %d is out of range", numeric)
		}
		return int32(numeric), nil
	}
	name := servicePort.TargetPort.StrVal
	if name == "" {
		return servicePort.Port, nil
	}
	for _, container := range pod.Spec.Containers {
		for _, containerPort := range container.Ports {
			if containerPort.Name == name {
				return containerPort.ContainerPort, nil
			}
		}
	}
	return 0, fmt.Errorf("pod %s has no port named %q", pod.Name, name)
}
