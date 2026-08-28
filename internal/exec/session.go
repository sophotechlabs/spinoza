package exec

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/safe"
)

const ShellPath = "/bin/sh"

type Request struct {
	Namespace string
	Pod       string
	Container string
	Command   []string
}

type Size struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type Options struct {
	Command []string
	Stdin   io.Reader
	Stdout  io.Writer
	Resize  <-chan Size
}

type Streamer interface {
	Stream(ctx context.Context, req Request, opts Options) error
}

type Session struct {
	stdin  *io.PipeWriter
	resize chan Size
	done   chan error
	cancel context.CancelFunc
	mu     sync.Mutex
	once   sync.Once
}

func start(ctx context.Context, streamer Streamer, req Request, stdout io.Writer, onDone func(error)) *Session {
	streamCtx, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	sess := &Session{
		stdin:  writer,
		resize: make(chan Size, 1),
		done:   make(chan error, 1),
		cancel: cancel,
	}
	safe.Go("exec into "+req.Namespace+"/"+req.Pod, func() {
		var err error
		completed := false
		defer func() {
			cancel()
			_ = reader.Close()
			if !completed {
				err = errors.New("the exec session ended unexpectedly")
			}
			onDone(err)
			sess.done <- err
		}()
		err = streamer.Stream(streamCtx, req, Options{
			Command: commandOf(req),
			Stdin:   reader,
			Stdout:  stdout,
			Resize:  sess.resize,
		})
		completed = true
	})
	return sess
}

func commandOf(req Request) []string {
	if len(req.Command) > 0 {
		return req.Command
	}
	return []string{ShellPath}
}

func (s *Session) Write(p []byte) (int, error) {
	return s.stdin.Write(p)
}

func (s *Session) Resize(size Size) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.resize:
	default:
	}
	s.resize <- size
}

func (s *Session) Done() <-chan error {
	return s.done
}

func (s *Session) Close() {
	s.once.Do(func() {
		_ = s.stdin.Close()
		s.cancel()
	})
}
