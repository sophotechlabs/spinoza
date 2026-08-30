package main

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/server"
)

type stubBackend struct {
	server.Backend

	stream *logs.Stream
	err    error
}

func (s stubBackend) Logs(_ context.Context, _ logs.Request) (*logs.Stream, error) {
	return s.stream, s.err
}

func openedStream(t *testing.T) *logs.Stream {
	t.Helper()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "web"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	stream, err := logs.Open(t.Context(), k8sfake.NewClientset(pod), logs.Request{
		Namespace: "web",
		Name:      "api",
		Container: "app",
		TailLines: 10,
	})
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}
	return stream
}

func TestTheReaderDrainsAStreamIntoLines(t *testing.T) {
	reader := logReader{Backend: stubBackend{stream: openedStream(t)}}

	lines, err := reader.LogLines(t.Context(), logs.Request{Namespace: "web", Name: "api"})
	if err != nil {
		t.Fatalf("reading the lines: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("the stream drained to nothing")
	}
}

func TestTheReaderPassesUpAStreamThatWouldNotOpen(t *testing.T) {
	wanted := errors.New("pods is forbidden")
	reader := logReader{Backend: stubBackend{err: wanted}}

	_, err := reader.LogLines(t.Context(), logs.Request{Namespace: "web", Name: "api"})

	if !errors.Is(err, wanted) {
		t.Fatalf("err = %v, want %v", err, wanted)
	}
}
