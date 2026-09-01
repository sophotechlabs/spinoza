package exec

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubStreamer struct {
	mu        sync.Mutex
	calls     int
	req       Request
	command   []string
	stdin     []byte
	stdinDone chan struct{}
	sizes     []Size
	entered   chan struct{}
	release   chan struct{}
	err       error
	echo      string
	drain     int
	gate      chan struct{}
}

func newStubStreamer() *stubStreamer {
	return &stubStreamer{
		entered:   make(chan struct{}, 1),
		release:   make(chan struct{}),
		stdinDone: make(chan struct{}),
	}
}

func (s *stubStreamer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *stubStreamer) Stream(ctx context.Context, req Request, opts Options) error {
	s.mu.Lock()
	s.calls++
	s.req = req
	s.command = opts.Command
	s.mu.Unlock()

	select {
	case s.entered <- struct{}{}:
	default:
	}

	if s.echo != "" {
		_, _ = opts.Stdout.Write([]byte(s.echo))
	}

	go func() {
		data, _ := io.ReadAll(opts.Stdin)
		s.mu.Lock()
		s.stdin = data
		s.mu.Unlock()
		close(s.stdinDone)
	}()

	if s.gate != nil {
		<-s.gate
	}

	for range s.drain {
		select {
		case size := <-opts.Resize:
			s.mu.Lock()
			s.sizes = append(s.sizes, size)
			s.mu.Unlock()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return s.err
}

func (s *stubStreamer) sizesSeen() []Size {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Size{}, s.sizes...)
}

func (s *stubStreamer) recorded() ([]byte, []Size, Request, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stdin, append([]Size{}, s.sizes...), s.req, s.command
}

func noop(error) {}

type panicStreamer struct{}

func (panicStreamer) Stream(context.Context, Request, Options) error {
	panic("streamer panicked")
}

func TestStartRunsShellAndForwardsStdin(t *testing.T) {
	streamer := newStubStreamer()
	var out bytes.Buffer
	sess := start(context.Background(), streamer, Request{Namespace: "flux-system", Pod: "web", Container: "app"}, &out, noop)

	<-streamer.entered
	_, err := sess.Write([]byte("uptime\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	sess.Close()
	close(streamer.release)

	select {
	case done := <-sess.Done():
		if done != nil {
			t.Fatalf("done: %v", done)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not finish")
	}

	<-streamer.stdinDone

	stdin, _, req, command := streamer.recorded()
	if string(stdin) != "uptime\n" {
		t.Fatalf("stdin = %q, want uptime", stdin)
	}
	if req.Pod != "web" {
		t.Fatalf("pod = %q", req.Pod)
	}
	if len(command) != 1 {
		t.Fatalf("command = %v", command)
	}
	if command[0] != ShellPath {
		t.Fatalf("command = %v", command)
	}
}

func TestStartReportsStreamError(t *testing.T) {
	streamer := newStubStreamer()
	streamer.err = errors.New("boom")
	var seen error
	sess := start(context.Background(), streamer, Request{}, io.Discard, func(err error) {
		seen = err
	})

	<-streamer.entered
	close(streamer.release)

	done := <-sess.Done()
	if done == nil {
		t.Fatal("expected an error")
	}
	if seen == nil {
		t.Fatal("onDone never saw the error")
	}
	sess.Close()
	sess.Close()
}

func TestStartReportsAStreamerPanicAsUnexpectedTermination(t *testing.T) {
	seen := make(chan error, 1)
	sess := start(t.Context(), panicStreamer{}, Request{}, io.Discard, func(err error) {
		seen <- err
	})

	done := <-sess.Done()

	if done == nil || !strings.Contains(done.Error(), "ended unexpectedly") {
		t.Fatalf("done = %v, want an unexpected termination", done)
	}
	callback := <-seen
	if callback == nil || callback.Error() != done.Error() {
		t.Fatalf("callback = %v, want %v", callback, done)
	}
}

func TestARequestedCommandReplacesTheDefaultShell(t *testing.T) {
	want := []string{"env", "sh", "-l"}
	got := commandOf(Request{Command: want})

	if !slices.Equal(got, want) {
		t.Fatalf("command = %v, want %v", got, want)
	}
}

func TestResizeKeepsOnlyTheNewestSize(t *testing.T) {
	streamer := newStubStreamer()
	streamer.drain = 1
	streamer.gate = make(chan struct{})
	sess := start(context.Background(), streamer, Request{}, io.Discard, noop)

	<-streamer.entered
	sess.Resize(Size{Cols: 80, Rows: 24})
	sess.Resize(Size{Cols: 120, Rows: 40})
	sess.Resize(Size{Cols: 200, Rows: 60})
	close(streamer.gate)
	close(streamer.release)
	<-sess.Done()

	sizes := streamer.sizesSeen()
	if len(sizes) != 1 {
		t.Fatalf("sizes = %v", sizes)
	}
	if sizes[0].Cols != 200 {
		t.Fatalf("cols = %d, want the newest size", sizes[0].Cols)
	}
	sess.Close()
}

func TestCloseStopsTheStream(t *testing.T) {
	streamer := newStubStreamer()
	sess := start(context.Background(), streamer, Request{}, io.Discard, noop)

	<-streamer.entered
	sess.Close()

	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("close did not end the session")
	}
}
