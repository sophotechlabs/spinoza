package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var ErrNoShell = errors.New("no shell")

type Images interface {
	ImageID(ctx context.Context, req Request) (string, error)
}

type cache struct {
	mu       sync.Mutex
	byDigest map[string]string
}

func newCache() *cache {
	return &cache{byDigest: map[string]string{}}
}

func (c *cache) get(digest string) string {
	if digest == "" {
		return api.ShellUnknown
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state, found := c.byDigest[digest]
	if !found {
		return api.ShellUnknown
	}
	return state
}

func (c *cache) set(digest string, state string) {
	if digest == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byDigest[digest] = state
}

type Service struct {
	streamer Streamer
	images   Images
	shells   *cache
}

func NewService(streamer Streamer, images Images) *Service {
	return &Service{streamer: streamer, images: images, shells: newCache()}
}

func (s *Service) Support(ctx context.Context, req Request) (api.ExecSupport, error) {
	digest, err := s.images.ImageID(ctx, req)
	if err != nil {
		return api.ExecSupport{}, err
	}
	return api.ExecSupport{
		Namespace: req.Namespace,
		Pod:       req.Pod,
		Container: req.Container,
		Image:     digest,
		Shell:     s.shells.get(digest),
	}, nil
}

func (s *Service) Start(ctx context.Context, req Request, stdout io.Writer) (*Session, error) {
	digest, err := s.images.ImageID(ctx, req)
	if err != nil {
		return nil, err
	}
	if s.shells.get(digest) == api.ShellAbsent {
		return nil, noShellError(req)
	}
	marker := &firstWrite{
		inner: stdout,
		mark: func() {
			s.shells.set(digest, api.ShellPresent)
		},
	}
	record := func(streamErr error) {
		if !IsMissingShell(streamErr) {
			return
		}
		s.shells.set(digest, api.ShellAbsent)
	}
	return start(ctx, s.streamer, req, marker, record), nil
}

func noShellError(req Request) error {
	return fmt.Errorf("%w: %s in %s/%s has no %s", ErrNoShell, containerLabel(req), req.Namespace, req.Pod, ShellPath)
}

func containerLabel(req Request) string {
	if req.Container == "" {
		return "the default container"
	}
	return req.Container
}

type firstWrite struct {
	inner  io.Writer
	mark   func()
	marked bool
}

func (f *firstWrite) Write(p []byte) (int, error) {
	if !f.marked {
		f.marked = true
		f.mark()
	}
	return f.inner.Write(p)
}

func IsMissingShell(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNoShell) {
		return true
	}
	text := err.Error()
	if !strings.Contains(text, ShellPath) {
		return false
	}
	if strings.Contains(text, "no such file or directory") {
		return true
	}
	return strings.Contains(text, "executable file not found")
}

type podImages struct {
	cs kubernetes.Interface
}

func NewImages(cs kubernetes.Interface) Images {
	return &podImages{cs: cs}
}

func (p *podImages) ImageID(ctx context.Context, req Request) (string, error) {
	pod, err := p.cs.CoreV1().Pods(req.Namespace).Get(ctx, req.Pod, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return imageIDFor(pod, req.Container)
}

func imageIDFor(pod *corev1.Pod, container string) (string, error) {
	name := container
	if name == "" {
		name = defaultContainer(pod)
	}
	for _, status := range allStatuses(pod) {
		if status.Name != name {
			continue
		}
		return status.ImageID, nil
	}
	if declares(pod, name) {
		return "", nil
	}
	return "", fmt.Errorf("pod %s/%s has no container %q", pod.Namespace, pod.Name, name)
}

func defaultContainer(pod *corev1.Pod) string {
	if len(pod.Spec.Containers) == 0 {
		return ""
	}
	return pod.Spec.Containers[0].Name
}

func allStatuses(pod *corev1.Pod) []corev1.ContainerStatus {
	out := make([]corev1.ContainerStatus, 0, len(pod.Status.ContainerStatuses))
	out = append(out, pod.Status.ContainerStatuses...)
	out = append(out, pod.Status.InitContainerStatuses...)
	out = append(out, pod.Status.EphemeralContainerStatuses...)
	return out
}

func declares(pod *corev1.Pod, name string) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == name {
			return true
		}
	}
	for _, container := range pod.Spec.InitContainers {
		if container.Name == name {
			return true
		}
	}
	for _, container := range pod.Spec.EphemeralContainers {
		if container.Name == name {
			return true
		}
	}
	return false
}
