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
	Selector  string
}

type Line struct {
	Pod  string
	Text string
}

type Stream struct {
	Lines    <-chan Line
	cancel   func()
	attached int
	matched  int
	mu       sync.Mutex
	err      error
}

// Attached and Matched differ when a workload has more pods than spinoza
// opens.
func (s *Stream) Attached() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attached
}

func (s *Stream) Matched() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.matched
}

func (s *Stream) setCounts(attached, matched int) {
	s.mu.Lock()
	s.attached = attached
	s.matched = matched
	s.mu.Unlock()
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
	if req.Selector != "" {
		return openMany(ctx, cs, req)
	}
	return openOne(ctx, cs, req)
}

func openOne(ctx context.Context, cs kubernetes.Interface, req Request) (*Stream, error) {
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

	lines := make(chan Line, lineBuffer)
	stream := &Stream{Lines: lines, cancel: shut, attached: 1, matched: 1}
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

func pump(ctx context.Context, r io.Reader, lines chan<- Line) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		case lines <- Line{Text: scanner.Text()}:
		}
	}
	return scanner.Err()
}
