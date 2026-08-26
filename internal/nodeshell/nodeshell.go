package nodeshell

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	DefaultNamespace = "kube-system"
	container        = "shell"
	managedBy        = "app.kubernetes.io/managed-by"
	nodeLabel        = "spinoza.sopho.tech/node"
	owner            = "spinoza"
)

var (
	startTimeout = 60 * time.Second
	pollEvery    = 500 * time.Millisecond
	removeGrace  = int64(0)
	// A privileged pod must not outlive the session that opened it, even if
	// spinoza dies without deleting it. The kubelet stops it either way.
	livesFor = int64((2 * time.Hour).Seconds())
)

// Enter is the argv that leaves the pod behind and lands in the node's own
// namespaces. The pod is only a way in; everything the shell sees is the host.
var Enter = []string{"nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "sh"}

type Service struct {
	cs        kubernetes.Interface
	image     string
	namespace string
	allow     func() bool
	perms     *access.Service
}

func NewService(
	cs kubernetes.Interface,
	image, namespace string,
	allow func() bool,
	perms *access.Service,
) *Service {
	if namespace == "" {
		namespace = DefaultNamespace
	}
	if allow == nil {
		allow = func() bool { return false }
	}
	return &Service{cs: cs, image: image, namespace: namespace, allow: allow, perms: perms}
}

func (s *Service) Support(ctx context.Context, node string) api.NodeShellSupport {
	enabled := s.allow()
	support := api.NodeShellSupport{
		Node:      node,
		Enabled:   enabled,
		Image:     s.image,
		Namespace: s.namespace,
	}
	if !enabled {
		support.Reason = "node shells are off; turn them on in Settings under Cluster, or start spinoza with --node-shell"
		return support
	}
	if node == "" {
		support.Reason = "no node was named"
		return support
	}
	// A node shell is a privileged pod on somebody's host. Unlike the buttons
	// elsewhere, one that cannot be asked about is not offered: better to say the
	// question could not be put than to hand over a shell nobody checked.
	decision := s.perms.Ask(ctx, access.Check{
		Verb:      "create",
		Resource:  "pods",
		Namespace: s.namespace,
	})
	if !decision.Answered {
		support.Reason = "could not check whether pods may be created in " + s.namespace
		if decision.Reason != "" {
			support.Reason += ": " + decision.Reason
		}
		return support
	}
	support.Allowed = decision.Allowed
	if !support.Allowed {
		support.Reason = "you may not create pods in " + s.namespace
		if decision.Reason != "" {
			support.Reason = decision.Reason
		}
	}
	return support
}

func (s *Service) Start(ctx context.Context, node string) (api.NodeShellSession, error) {
	if !s.allow() {
		return api.NodeShellSession{}, fmt.Errorf("%w: node shells are off", api.ErrInternal)
	}
	if node == "" {
		return api.NodeShellSession{}, fmt.Errorf("%w: no node was named", api.ErrInternal)
	}
	created, err := s.cs.CoreV1().Pods(s.namespace).Create(ctx, s.pod(node), metav1.CreateOptions{})
	if err != nil {
		return api.NodeShellSession{}, err
	}
	waitErr := s.waitRunning(ctx, created.Name)
	if waitErr != nil {
		s.Remove(ctx, created.Name)
		return api.NodeShellSession{}, waitErr
	}
	return api.NodeShellSession{
		Namespace: s.namespace,
		Pod:       created.Name,
		Container: container,
		Node:      node,
		Image:     s.image,
	}, nil
}

func (s *Service) Remove(ctx context.Context, pod string) {
	if pod == "" {
		return
	}
	_ = s.cs.CoreV1().Pods(s.namespace).Delete(ctx, pod, metav1.DeleteOptions{GracePeriodSeconds: &removeGrace})
}

func (s *Service) waitRunning(ctx context.Context, name string) error {
	deadline := time.Now().Add(startTimeout)
	for {
		pod, err := s.cs.CoreV1().Pods(s.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if pod.Status.Phase == corev1.PodRunning {
			return nil
		}
		if pod.Status.Phase == corev1.PodFailed {
			return fmt.Errorf("%w: the node shell pod failed to start: %s", api.ErrInternal, pod.Status.Reason)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: the node shell pod did not start within %s", api.ErrInternal, startTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollEvery):
		}
	}
}

func (s *Service) pod(node string) *corev1.Pod {
	privileged := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "spinoza-node-shell-",
			Namespace:    s.namespace,
			Labels:       map[string]string{managedBy: owner, nodeLabel: node},
		},
		Spec: corev1.PodSpec{
			ActiveDeadlineSeconds: &livesFor,
			NodeName:              node,
			HostPID:               true,
			HostIPC:               true,
			HostNetwork:           true,
			RestartPolicy:         corev1.RestartPolicyNever,
			Tolerations:           []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			Containers: []corev1.Container{{
				Name:            container,
				Image:           s.image,
				Command:         []string{"sleep"},
				Args:            []string{"infinity"},
				SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
			}},
		},
	}
}
