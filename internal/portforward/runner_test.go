package portforward

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

type errorStream struct {
	headers http.Header
}

func (e *errorStream) Read([]byte) (int, error) { return 0, io.EOF }
func (e *errorStream) Write(p []byte) (int, error) {
	return len(p), nil
}
func (e *errorStream) Close() error         { return nil }
func (e *errorStream) Reset() error         { return nil }
func (e *errorStream) Headers() http.Header { return e.headers }
func (e *errorStream) Identifier() uint32   { return 1 }

type pipeStream struct {
	net.Conn

	headers http.Header
}

func (p *pipeStream) Reset() error         { return p.Close() }
func (p *pipeStream) Headers() http.Header { return p.headers }
func (p *pipeStream) Identifier() uint32   { return 2 }

type fakeConnection struct {
	closed chan bool

	mu      sync.Mutex
	headers []http.Header
}

func (c *fakeConnection) CreateStream(headers http.Header) (streamhttp.Stream, error) {
	c.mu.Lock()
	c.headers = append(c.headers, headers.Clone())
	c.mu.Unlock()

	if headers.Get(corev1.StreamType) == corev1.StreamTypeError {
		return &errorStream{headers: headers}, nil
	}
	client, server := net.Pipe()
	go func() {
		defer func() { _ = server.Close() }()
		_, _ = io.Copy(server, server)
	}()
	return &pipeStream{Conn: client, headers: headers}, nil
}

func (c *fakeConnection) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return nil
}

func (c *fakeConnection) CloseChan() <-chan bool             { return c.closed }
func (c *fakeConnection) SetIdleTimeout(time.Duration)       {}
func (c *fakeConnection) RemoveStreams(...streamhttp.Stream) {}

func (c *fakeConnection) portHeaders() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []string{}
	for _, h := range c.headers {
		out = append(out, h.Get(corev1.PortHeader))
	}
	return out
}

type fakeDialer struct {
	conn *fakeConnection
	err  error
}

func (d *fakeDialer) Dial(protocols ...string) (streamhttp.Connection, string, error) {
	if d.err != nil {
		return nil, "", d.err
	}
	return d.conn, protocols[0], nil
}

func runnerWith(dialer streamhttp.Dialer, err error) *streamRunner {
	return &streamRunner{
		dialerFor: func(string, string) (streamhttp.Dialer, error) {
			if err != nil {
				return nil, err
			}
			return dialer, nil
		},
	}
}

func TestRunForwardsTrafficThroughALocalListener(t *testing.T) {
	conn := &fakeConnection{closed: make(chan bool)}
	runner := runnerWith(&fakeDialer{conn: conn}, nil)
	ready := make(chan int32, 1)
	stop := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- runner.Run(context.Background(), "flux-system", "web", 8080, ready, stop)
	}()

	var local int32
	select {
	case local = <-ready:
	case err := <-done:
		t.Fatalf("run ended before ready: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for the forward to become ready")
	}
	if local == 0 {
		t.Fatalf("local port was not reported")
	}

	client, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", local))
	if err != nil {
		t.Fatalf("dial local port: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if err := client.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echoed %q, want ping", buf)
	}

	ports := conn.portHeaders()
	if len(ports) < 2 {
		t.Fatalf("expected an error stream and a data stream, got %v", ports)
	}
	for _, port := range ports {
		if port != "8080" {
			t.Fatalf("stream opened for port %q, want 8080", port)
		}
	}

	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v after stop", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("run did not return after stop")
	}
}

func TestRunStopsWithoutAnyTraffic(t *testing.T) {
	runner := runnerWith(&fakeDialer{conn: &fakeConnection{closed: make(chan bool)}}, nil)
	ready := make(chan int32, 1)
	stop := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- runner.Run(context.Background(), "flux-system", "web", 9090, ready, stop)
	}()

	<-ready
	close(stop)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("run did not return after stop")
	}
}

func TestRunSurfacesADialerBuildFailure(t *testing.T) {
	runner := runnerWith(nil, errors.New("no kubeconfig"))

	err := runner.Run(context.Background(), "flux-system", "web", 8080, make(chan int32, 1), make(chan struct{}))

	if err == nil {
		t.Fatalf("expected the dialer failure to surface")
	}
}

func TestRunSurfacesADialFailure(t *testing.T) {
	runner := runnerWith(&fakeDialer{err: errors.New("upgrade refused")}, nil)
	stop := make(chan struct{})
	defer close(stop)

	err := runner.Run(context.Background(), "flux-system", "web", 8080, make(chan int32, 1), stop)

	if err == nil {
		t.Fatalf("expected the dial failure to surface")
	}
}

func TestAnnounceGivesUpWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan int32, 1)
	done := make(chan struct{})

	go func() {
		announce(ctx, nil, make(chan struct{}), ready)
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("announce did not return when the context ended")
	}
	if len(ready) != 0 {
		t.Fatalf("announce reported a port after the context ended")
	}
}

func TestFallbackDialerIsBuiltForAValidConfig(t *testing.T) {
	endpoint, err := url.Parse("https://example.test/api/v1/namespaces/x/pods/y/portforward")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	dialer, err := fallbackDialer(&restclient.Config{Host: "https://example.test"}, endpoint)
	if err != nil {
		t.Fatalf("fallbackDialer: %v", err)
	}
	if dialer == nil {
		t.Fatalf("dialer is nil")
	}
}

func TestFallbackDialerRejectsABrokenConfig(t *testing.T) {
	endpoint, err := url.Parse("https://example.test/portforward")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	config := &restclient.Config{
		Host: "https://example.test",
		TLSClientConfig: restclient.TLSClientConfig{
			CertFile: "/nonexistent/cert.pem",
			KeyFile:  "/nonexistent/key.pem",
		},
	}

	_, err = fallbackDialer(config, endpoint)

	if err == nil {
		t.Fatalf("expected a transport build failure")
	}
}

func TestNewRunnerBuildsADialerFromTheClientset(t *testing.T) {
	config := &restclient.Config{Host: "https://example.test"}
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	runner, ok := NewRunner(cs, config).(*streamRunner)
	if !ok {
		t.Fatalf("NewRunner did not return a streamRunner")
	}

	dialer, err := runner.dialerFor("flux-system", "web")
	if err != nil {
		t.Fatalf("dialerFor: %v", err)
	}
	if dialer == nil {
		t.Fatalf("dialer is nil")
	}
}

func TestAnnounceIgnoresAForwarderThatIsNotReady(t *testing.T) {
	forwarder, err := portforward.NewForStreaming(
		&fakeDialer{conn: &fakeConnection{closed: make(chan bool)}},
		[]string{"0:8080"},
		make(chan struct{}),
		make(chan struct{}),
		io.Discard,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("new forwarder: %v", err)
	}
	ready := make(chan int32, 1)
	closed := make(chan struct{})
	close(closed)

	announce(context.Background(), forwarder, closed, ready)

	if len(ready) != 0 {
		t.Fatalf("announce reported a port for a forwarder that never bound one")
	}
}

func TestShouldFallbackOnlyForUpgradeFailures(t *testing.T) {
	if shouldFallback(errors.New("connection refused")) {
		t.Fatalf("a plain error must not trigger the spdy fallback")
	}
	if !shouldFallback(&streamhttp.UpgradeFailureError{Cause: errors.New("no")}) {
		t.Fatalf("an upgrade failure must trigger the spdy fallback")
	}
}
