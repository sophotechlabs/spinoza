package exec

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubImages struct {
	digest string
	err    error
}

func (s *stubImages) ImageID(context.Context, Request) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.digest, nil
}

func podRequest() Request {
	return Request{Namespace: "monitoring", Pod: "loki-0", Container: "loki"}
}

func TestSupportStartsUnknown(t *testing.T) {
	svc := NewService(newStubStreamer(), &stubImages{digest: "sha256:abc"})
	support, err := svc.Support(context.Background(), podRequest())
	if err != nil {
		t.Fatalf("support: %v", err)
	}
	if support.Shell != api.ShellUnknown {
		t.Fatalf("shell = %q", support.Shell)
	}
	if support.Image != "sha256:abc" {
		t.Fatalf("image = %q", support.Image)
	}
	if support.Pod != "loki-0" {
		t.Fatalf("pod = %q", support.Pod)
	}
}

func TestSupportReportsLookupFailure(t *testing.T) {
	svc := NewService(newStubStreamer(), &stubImages{err: errors.New("nope")})
	_, err := svc.Support(context.Background(), podRequest())
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestStartMarksTheImagePresentOnFirstOutput(t *testing.T) {
	streamer := newStubStreamer()
	streamer.echo = "/ # "
	svc := NewService(streamer, &stubImages{digest: "sha256:shelled"})

	var out bytes.Buffer
	sess, err := svc.Start(context.Background(), podRequest(), &out)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	<-streamer.entered
	close(streamer.release)
	<-sess.Done()
	sess.Close()

	support, _ := svc.Support(context.Background(), podRequest())
	if support.Shell != api.ShellPresent {
		t.Fatalf("shell = %q", support.Shell)
	}
	if out.String() != "/ # " {
		t.Fatalf("out = %q", out.String())
	}
}

func TestStartMarksTheImageAbsentAndRefusesTheNextRun(t *testing.T) {
	streamer := newStubStreamer()
	streamer.err = errors.New(`exec: "/bin/sh": stat /bin/sh: no such file or directory`)
	svc := NewService(streamer, &stubImages{digest: "sha256:distroless"})

	sess, err := svc.Start(context.Background(), podRequest(), io.Discard)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	<-streamer.entered
	close(streamer.release)
	<-sess.Done()
	sess.Close()

	support, _ := svc.Support(context.Background(), podRequest())
	if support.Shell != api.ShellAbsent {
		t.Fatalf("shell = %q", support.Shell)
	}

	_, again := svc.Start(context.Background(), podRequest(), io.Discard)
	if !errors.Is(again, ErrNoShell) {
		t.Fatalf("second start = %v, want ErrNoShell", again)
	}
	if streamer.count() != 1 {
		t.Fatalf("streamer ran %d times, want 1", streamer.count())
	}
}

func TestStartNamesTheDefaultContainerWhenNoneWasAsked(t *testing.T) {
	streamer := newStubStreamer()
	streamer.err = errors.New(`exec: "/bin/sh": stat /bin/sh: no such file or directory`)
	svc := NewService(streamer, &stubImages{digest: "sha256:distroless"})
	req := Request{Namespace: "monitoring", Pod: "loki-0"}

	sess, _ := svc.Start(context.Background(), req, io.Discard)
	<-streamer.entered
	close(streamer.release)
	<-sess.Done()
	sess.Close()

	_, err := svc.Start(context.Background(), req, io.Discard)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !contains(err.Error(), "the default container") {
		t.Fatalf("message = %q", err.Error())
	}
	if !contains(err.Error(), ShellPath) {
		t.Fatalf("message = %q, want the path that was tried", err.Error())
	}
}

func TestStartReportsLookupFailure(t *testing.T) {
	svc := NewService(newStubStreamer(), &stubImages{err: errors.New("nope")})
	_, err := svc.Start(context.Background(), podRequest(), io.Discard)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestUnknownDigestIsNeverCached(t *testing.T) {
	streamer := newStubStreamer()
	streamer.err = errors.New(`exec: "/bin/sh": stat /bin/sh: no such file or directory`)
	svc := NewService(streamer, &stubImages{digest: ""})

	sess, err := svc.Start(context.Background(), podRequest(), io.Discard)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	<-streamer.entered
	close(streamer.release)
	<-sess.Done()
	sess.Close()

	support, _ := svc.Support(context.Background(), podRequest())
	if support.Shell != api.ShellUnknown {
		t.Fatalf("shell = %q, want it left unknown without a digest", support.Shell)
	}
}

func TestIsMissingShell(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"sentinel", ErrNoShell, true},
		{"stat", errors.New(`exec: "/bin/sh": stat /bin/sh: no such file or directory`), true},
		{"not in path", errors.New(`exec: "/bin/sh": executable file not found in $PATH`), true},
		{"other path missing", errors.New(`stat /usr/bin/psql: no such file or directory`), false},
		{"exit code", errors.New("command terminated with exit code 1"), false},
		{"shell named but unrelated", errors.New("/bin/sh: permission denied"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsMissingShell(tc.err)
			if got != tc.want {
				t.Fatalf("IsMissingShell(%v) = %v", tc.err, got)
			}
		})
	}
}

func TestImageIDReadsTheContainerStatus(t *testing.T) {
	cs := k8sfake.NewClientset(pod())
	images := NewImages(cs)

	digest, err := images.ImageID(context.Background(), podRequest())
	if err != nil {
		t.Fatalf("image id: %v", err)
	}
	if digest != "docker.io/grafana/loki@sha256:1111" {
		t.Fatalf("digest = %q", digest)
	}
}

func TestImageIDFallsBackToTheFirstContainer(t *testing.T) {
	cs := k8sfake.NewClientset(pod())
	images := NewImages(cs)

	digest, err := images.ImageID(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0"})
	if err != nil {
		t.Fatalf("image id: %v", err)
	}
	if digest != "docker.io/grafana/loki@sha256:1111" {
		t.Fatalf("digest = %q", digest)
	}
}

func TestImageIDFindsInitAndEphemeralContainers(t *testing.T) {
	cs := k8sfake.NewClientset(pod())
	images := NewImages(cs)

	init, err := images.ImageID(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0", Container: "chown"})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if init != "busybox@sha256:2222" {
		t.Fatalf("init digest = %q", init)
	}

	debug, err := images.ImageID(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0", Container: "debugger"})
	if err != nil {
		t.Fatalf("ephemeral: %v", err)
	}
	if debug != "busybox@sha256:3333" {
		t.Fatalf("ephemeral digest = %q", debug)
	}
}

func TestImageIDIsEmptyBeforeTheContainerHasStatus(t *testing.T) {
	cs := k8sfake.NewClientset(pod())
	images := NewImages(cs)

	digest, err := images.ImageID(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0", Container: "sidecar"})
	if err != nil {
		t.Fatalf("image id: %v", err)
	}
	if digest != "" {
		t.Fatalf("digest = %q, want empty until the status appears", digest)
	}
}

func TestImageIDRejectsAnUnknownContainer(t *testing.T) {
	cs := k8sfake.NewClientset(pod())
	images := NewImages(cs)

	_, err := images.ImageID(context.Background(), Request{Namespace: "monitoring", Pod: "loki-0", Container: "ghost"})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestImageIDReportsAMissingPod(t *testing.T) {
	images := NewImages(k8sfake.NewClientset())
	_, err := images.ImageID(context.Background(), podRequest())
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestImageIDIsEmptyForDeclaredButUnstartedContainers(t *testing.T) {
	declared := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "monitoring", Name: "pending"},
		Spec: corev1.PodSpec{
			Containers:          []corev1.Container{{Name: "app"}},
			InitContainers:      []corev1.Container{{Name: "chown"}},
			EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debugger"}}},
		},
	}
	for _, name := range []string{"chown", "debugger"} {
		digest, err := imageIDFor(declared, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if digest != "" {
			t.Fatalf("%s digest = %q", name, digest)
		}
	}
}

func TestImageIDForAPodWithoutContainers(t *testing.T) {
	empty := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "monitoring", Name: "blank"}}
	_, err := imageIDFor(empty, "")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func pod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "monitoring", Name: "loki-0"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "loki"},
				{Name: "sidecar"},
			},
			InitContainers:      []corev1.Container{{Name: "chown"}},
			EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debugger"}}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses:          []corev1.ContainerStatus{{Name: "loki", ImageID: "docker.io/grafana/loki@sha256:1111"}},
			InitContainerStatuses:      []corev1.ContainerStatus{{Name: "chown", ImageID: "busybox@sha256:2222"}},
			EphemeralContainerStatuses: []corev1.ContainerStatus{{Name: "debugger", ImageID: "busybox@sha256:3333"}},
		},
	}
}

func contains(haystack string, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}
