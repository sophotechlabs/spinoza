package debugcontainer

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	DefaultImage   = "busybox:1.37"
	DefaultProfile = "general"

	namePrefix     = "spinoza-debug-"
	mirrorPodKey   = "kubernetes.io/config.mirror"
	defaultTimeout = 90 * time.Second
	defaultPoll    = 500 * time.Millisecond
)

var ErrUnavailable = errors.New("debug containers are unavailable")

const sysadminProfile = "sysadmin"

var profiles = []string{"general", "baseline", "restricted", "netadmin", sysadminProfile, "legacy"}

func profileOf(spec *corev1.EphemeralContainer) string {
	security := spec.SecurityContext
	if security == nil {
		return ""
	}
	if security.Privileged != nil && *security.Privileged {
		return sysadminProfile
	}
	added := addedCapabilities(security)
	if slices.Contains(added, "NET_ADMIN") && slices.Contains(added, "NET_RAW") {
		return "netadmin"
	}
	if slices.Contains(added, "SYS_PTRACE") {
		return "general"
	}
	if security.RunAsNonRoot != nil && *security.RunAsNonRoot {
		return "restricted"
	}
	return ""
}

func addedCapabilities(security *corev1.SecurityContext) []string {
	if security.Capabilities == nil {
		return nil
	}
	out := make([]string, 0, len(security.Capabilities.Add))
	for _, capability := range security.Capabilities.Add {
		out = append(out, string(capability))
	}
	return out
}

var nameFormat = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

var startFailures = map[string]bool{
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"InvalidImageName":           true,
	"CreateContainerError":       true,
	"CreateContainerConfigError": true,
}

type Request struct {
	Namespace string
	Pod       string
	Container string
	Profile   string
}

type Runner interface {
	Run(ctx context.Context, args []string) error
}

type Service struct {
	runner  Runner
	cs      kubernetes.Interface
	image   string
	kubeCtx string
	timeout time.Duration
	poll    time.Duration
}

func NewService(runner Runner, cs kubernetes.Interface, image, kubeCtx string) *Service {
	if image == "" {
		image = DefaultImage
	}
	return &Service{
		runner:  runner,
		cs:      cs,
		image:   image,
		kubeCtx: kubeCtx,
		timeout: defaultTimeout,
		poll:    defaultPoll,
	}
}

func Supported(profile string) bool {
	return slices.Contains(profiles, profile)
}

func (s *Service) Allowed(ctx context.Context, namespace, pod string) api.DebugSupport {
	support := api.DebugSupport{Namespace: namespace, Pod: pod, Allowed: true, Image: s.image}
	review := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace:   namespace,
				Name:        pod,
				Verb:        "patch",
				Resource:    "pods",
				Subresource: "ephemeralcontainers",
			},
		},
	}
	result, err := s.cs.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return support
	}
	support.Allowed = result.Status.Allowed
	support.Reason = result.Status.Reason
	return support
}

func (s *Service) Ensure(ctx context.Context, req Request) (api.DebugSession, error) {
	profile := req.Profile
	if profile == "" {
		profile = DefaultProfile
	}
	if !Supported(profile) {
		return api.DebugSession{}, fmt.Errorf("unknown debug profile %q", profile)
	}

	pod, err := s.cs.CoreV1().Pods(req.Namespace).Get(ctx, req.Pod, metav1.GetOptions{})
	if err != nil {
		return api.DebugSession{}, err
	}
	err = admits(pod, req)
	if err != nil {
		return api.DebugSession{}, err
	}

	existing, runningImage, found := runningDebugContainer(pod)
	if found {
		if req.Profile == sysadminProfile && !privileged(existing) {
			return api.DebugSession{}, fmt.Errorf(
				"%s already has debug container %q, which is not privileged; an ephemeral container cannot be changed once it exists, so attach to it or recreate the pod",
				req.Pod, existing.Name,
			)
		}
		return api.DebugSession{
			Container: existing.Name,
			Image:     imageOf(existing, runningImage),
			Profile:   profileOf(existing),
			Target:    existing.TargetContainerName,
		}, nil
	}

	name := nextName(pod)
	runErr := s.runner.Run(ctx, s.args(req, name, profile))
	if runErr != nil {
		return api.DebugSession{}, runErr
	}

	waitErr := s.waitRunning(ctx, req, name)
	if waitErr != nil {
		return api.DebugSession{}, waitErr
	}
	return api.DebugSession{
		Container: name,
		Created:   true,
		Image:     s.image,
		Profile:   profile,
		Target:    req.Container,
	}, nil
}

func (s *Service) args(req Request, name, profile string) []string {
	args := []string{
		"debug", req.Pod,
		"--namespace", req.Namespace,
		"--image", s.image,
		"--container", name,
		"--profile", profile,
		"--attach=false",
		"--stdin",
	}
	if req.Container != "" {
		args = append(args, "--target", req.Container)
	}
	if s.kubeCtx != "" {
		args = append(args, "--context", s.kubeCtx)
	}
	return args
}

func (s *Service) waitRunning(ctx context.Context, req Request, name string) error {
	deadline := time.Now().Add(s.timeout)
	reason := ""
	for time.Now().Before(deadline) {
		pod, err := s.cs.CoreV1().Pods(req.Namespace).Get(ctx, req.Pod, metav1.GetOptions{})
		if err != nil {
			return err
		}
		running, waiting, startErr := progressOf(pod, name)
		if startErr != nil {
			return startErr
		}
		if running {
			return nil
		}
		if waiting != "" {
			reason = waiting
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.poll):
		}
	}
	return neverStarted(name, s.timeout, reason)
}

func progressOf(pod *corev1.Pod, name string) (running bool, waiting string, err error) {
	status, found := statusOf(pod, name)
	if !found {
		return false, "", nil
	}
	if status.State.Running != nil {
		return true, "", nil
	}
	if status.State.Terminated != nil {
		return false, "", fmt.Errorf("debug container %s exited immediately: %s", name, status.State.Terminated.Reason)
	}
	if status.State.Waiting == nil {
		return false, "", nil
	}
	reason := status.State.Waiting.Reason
	if startFailures[reason] {
		return false, reason, fmt.Errorf("debug container %s could not start: %s — %s", name, reason, status.State.Waiting.Message)
	}
	return false, reason, nil
}

func neverStarted(name string, timeout time.Duration, reason string) error {
	if reason == "" {
		return fmt.Errorf("debug container %s did not start within %s", name, timeout)
	}
	return fmt.Errorf("debug container %s did not start within %s, last state: %s", name, timeout, reason)
}

func admits(pod *corev1.Pod, req Request) error {
	if !nameFormat.MatchString(req.Namespace) || !nameFormat.MatchString(req.Pod) {
		return errors.New("namespace and pod must be valid kubernetes names")
	}
	if req.Container != "" && !nameFormat.MatchString(req.Container) {
		return errors.New("container must be a valid kubernetes name")
	}
	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("pod %s/%s is %s, debug containers need a running pod", pod.Namespace, pod.Name, pod.Status.Phase)
	}
	_, mirror := pod.Annotations[mirrorPodKey]
	if mirror {
		return fmt.Errorf("pod %s/%s is a static pod, which cannot take debug containers", pod.Namespace, pod.Name)
	}
	if req.Container == "" {
		return nil
	}
	for _, container := range pod.Spec.Containers {
		if container.Name == req.Container {
			return nil
		}
	}
	return fmt.Errorf("pod %s/%s has no container %q to target", pod.Namespace, pod.Name, req.Container)
}

func runningDebugContainer(pod *corev1.Pod) (*corev1.EphemeralContainer, string, bool) {
	for _, status := range pod.Status.EphemeralContainerStatuses {
		if !strings.HasPrefix(status.Name, namePrefix) {
			continue
		}
		if status.State.Running == nil {
			continue
		}
		spec := specFor(pod, status.Name)
		if spec == nil {
			continue
		}
		return spec, status.Image, true
	}
	return nil, "", false
}

func specFor(pod *corev1.Pod, name string) *corev1.EphemeralContainer {
	for i := range pod.Spec.EphemeralContainers {
		if pod.Spec.EphemeralContainers[i].Name == name {
			return &pod.Spec.EphemeralContainers[i]
		}
	}
	return nil
}

func privileged(spec *corev1.EphemeralContainer) bool {
	if spec.SecurityContext == nil {
		return false
	}
	if spec.SecurityContext.Privileged == nil {
		return false
	}
	return *spec.SecurityContext.Privileged
}

func imageOf(spec *corev1.EphemeralContainer, running string) string {
	if spec.Image != "" {
		return spec.Image
	}
	return running
}

func nextName(pod *corev1.Pod) string {
	highest := 0
	for _, container := range pod.Spec.EphemeralContainers {
		index, ok := indexOf(container.Name)
		if !ok {
			continue
		}
		if index > highest {
			highest = index
		}
	}
	return namePrefix + strconv.Itoa(highest+1)
}

func indexOf(name string) (int, bool) {
	suffix, found := strings.CutPrefix(name, namePrefix)
	if !found {
		return 0, false
	}
	index, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, false
	}
	return index, true
}

func statusOf(pod *corev1.Pod, name string) (corev1.ContainerStatus, bool) {
	for _, status := range pod.Status.EphemeralContainerStatuses {
		if status.Name == name {
			return status, true
		}
	}
	return corev1.ContainerStatus{}, false
}
