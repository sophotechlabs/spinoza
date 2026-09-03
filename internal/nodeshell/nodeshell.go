package nodeshell

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/imagepin"
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
	livesFor     = int64((2 * time.Hour).Seconds())
)

const removeEvery = 250 * time.Millisecond

const startCleanupTimeout = 15 * time.Second

var Enter = []string{"nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "sh"}

type Service struct {
	cs          kubernetes.Interface
	image       string
	namespace   string
	allow       func() bool
	perms       *access.Service
	removeEvery time.Duration
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
	return &Service{
		cs:          cs,
		image:       image,
		namespace:   namespace,
		allow:       allow,
		perms:       perms,
		removeEvery: removeEvery,
	}
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
	if !imagepin.Valid(s.image) {
		support.Reason = "node shells require an image pinned by sha256 digest"
		return support
	}
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
	if !imagepin.Valid(s.image) {
		return api.NodeShellSession{}, fmt.Errorf("%w: node shells require an image pinned by sha256 digest", api.ErrInternal)
	}
	created, err := s.cs.CoreV1().Pods(s.namespace).Create(ctx, s.pod(node), metav1.CreateOptions{})
	if err != nil {
		return api.NodeShellSession{}, err
	}
	waitErr := s.waitRunning(ctx, created.Name)
	if waitErr != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startCleanupTimeout)
		defer cancel()
		removeErr := s.Remove(cleanupCtx, created.Name)
		if removeErr != nil {
			return api.NodeShellSession{}, errors.Join(waitErr, removeErr)
		}
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

func (s *Service) Remove(ctx context.Context, pod string) error {
	if pod == "" {
		return nil
	}
	for {
		err := s.cs.CoreV1().Pods(s.namespace).Delete(
			ctx,
			pod,
			metav1.DeleteOptions{GracePeriodSeconds: &removeGrace},
		)
		if err == nil || apierrors.IsNotFound(err) {
			return nil
		}
		if permanentDeleteError(err) {
			return fmt.Errorf("removing node shell pod %s: %w", pod, err)
		}
		timer := time.NewTimer(s.removeEvery)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return fmt.Errorf("removing node shell pod %s: %w", pod, errors.Join(err, ctx.Err()))
		case <-timer.C:
		}
	}
}

func permanentDeleteError(err error) bool {
	return apierrors.IsBadRequest(err) ||
		apierrors.IsForbidden(err) ||
		apierrors.IsInvalid(err) ||
		apierrors.IsMethodNotSupported(err) ||
		apierrors.IsUnauthorized(err)
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
