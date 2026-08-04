package logs

import (
	"bufio"
	"context"
	"io"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	lineBuffer   = 256
	maxLineBytes = 1 << 20
)

type Request struct {
	Namespace string
	Name      string
	Container string
	TailLines int64
	Follow    bool
}

type Stream struct {
	Lines  <-chan string
	cancel func()
	mu     sync.Mutex
	err    error
}

func (s *Stream) Close() {
	s.cancel()
}

func (s *Stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Stream) fail(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func Open(ctx context.Context, cs kubernetes.Interface, req Request) (*Stream, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	rc, err := cs.CoreV1().Pods(req.Namespace).GetLogs(req.Name, optionsFor(req)).Stream(streamCtx)
	if err != nil {
		cancel()
		return nil, err
	}

	var once sync.Once
	shut := func() {
		once.Do(func() {
			cancel()
			_ = rc.Close()
		})
	}

	lines := make(chan string, lineBuffer)
	stream := &Stream{Lines: lines, cancel: shut}
	safe.Go("streaming logs for "+req.Namespace+"/"+req.Name, func() {
		defer close(lines)
		defer shut()
		readErr := pump(streamCtx, rc, lines)
		if streamCtx.Err() != nil {
			return
		}
		stream.fail(readErr)
	})

	return stream, nil
}

func optionsFor(req Request) *corev1.PodLogOptions {
	opts := &corev1.PodLogOptions{
		Container:  req.Container,
		Follow:     req.Follow,
		Timestamps: false,
	}
	if req.TailLines > 0 {
		tail := req.TailLines
		opts.TailLines = &tail
	}
	return opts
}

func pump(ctx context.Context, r io.Reader, lines chan<- string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		case lines <- scanner.Text():
		}
	}
	return scanner.Err()
}
