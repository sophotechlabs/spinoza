package reach

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// roundTripper is a transport that answers however a test wants it to.
type roundTripper struct {
	resp *http.Response
	err  error
}

func (r *roundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return r.resp, r.err
}

func through(sink *Sink, resp *http.Response, err error) (*http.Response, error) {
	wrapped := sink.Wrap(&roundTripper{resp: resp, err: err})
	request, _ := http.NewRequest(http.MethodGet, "https://cluster.test/api", http.NoBody)
	return wrapped.RoundTrip(request)
}

func fired(t *testing.T, sink *Sink) bool {
	t.Helper()
	select {
	case <-sink.Changed():
		return true
	case <-time.After(time.Second):
		return false
	}
}

func quiet(t *testing.T, sink *Sink) bool {
	t.Helper()
	select {
	case <-sink.Changed():
		return false
	case <-time.After(50 * time.Millisecond):
		return true
	}
}

func TestARequestThatCameBackWithNothingSaysTheClusterIsGone(t *testing.T) {
	sink := New()

	_, _ = through(sink, nil, errors.New("dial tcp 10.0.0.1:6443: connect: connection refused"))

	answering, reason := sink.State()
	if answering {
		t.Fatal("a request that never got a reply left the cluster looking fine")
	}
	if reason != "dial tcp 10.0.0.1:6443: connect: connection refused" {
		t.Fatalf("reason = %q, want what the transport said", reason)
	}
	if !fired(t, sink) {
		t.Fatal("nobody was told the answer changed")
	}
}

// Even a refusal is a reply, and a reply means the cluster is there.
func TestAReplyOfAnyKindSaysTheClusterIsThere(t *testing.T) {
	sink := New()
	_, _ = through(sink, nil, errors.New("connection refused"))
	<-sink.Changed()

	_, _ = through(sink, &http.Response{StatusCode: http.StatusForbidden}, nil)

	answering, reason := sink.State()
	if !answering {
		t.Fatal("a cluster that answered 403 was still counted as gone")
	}
	if reason != "" {
		t.Fatalf("reason = %q, want it forgotten", reason)
	}
	if !fired(t, sink) {
		t.Fatal("nobody was told the cluster came back")
	}
}

// A window that closes cancels everything it had open. That is not an outage.
func TestARequestTheCallerGaveUpOnIsIgnored(t *testing.T) {
	sink := New()

	_, _ = through(sink, nil, fmt.Errorf("reading body: %w", context.Canceled))

	answering, _ := sink.State()
	if !answering {
		t.Fatal("a request the caller dropped was read as an outage")
	}
	if !quiet(t, sink) {
		t.Fatal("a request the caller dropped was announced")
	}
}

// A deadline that ran out is the cluster taking longer than spinoza will wait,
// which is what a window needs to know about.
func TestARequestThatRanOutOfTimeIsReported(t *testing.T) {
	sink := New()

	_, _ = through(sink, nil, fmt.Errorf("awaiting headers: %w", context.DeadlineExceeded))

	answering, reason := sink.State()
	if answering {
		t.Fatal("a request that timed out left the cluster looking fine")
	}
	if reason == "" {
		t.Fatal("the timeout was reported without saying what happened")
	}
}

func TestTheSameAnswerTwiceIsSaidOnce(t *testing.T) {
	sink := New()
	_, _ = through(sink, nil, errors.New("connection refused"))
	<-sink.Changed()

	_, _ = through(sink, nil, errors.New("connection refused"))

	if !quiet(t, sink) {
		t.Fatal("the same answer was announced twice")
	}
}

func TestADifferentReasonIsSaidAgain(t *testing.T) {
	sink := New()
	_, _ = through(sink, nil, errors.New("connection refused"))
	<-sink.Changed()

	_, _ = through(sink, nil, errors.New("no such host"))

	if !fired(t, sink) {
		t.Fatal("a cluster failing differently was not mentioned")
	}
	_, reason := sink.State()
	if reason != "no such host" {
		t.Fatalf("reason = %q, want the newer one", reason)
	}
}

// The wrapper is on the path of every request, so it must hand back exactly
// what the transport gave it.
func TestTheResponseIsHandedOnUntouched(t *testing.T) {
	sink := New()
	want := &http.Response{StatusCode: http.StatusTeapot}

	got, err := through(sink, want, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != want {
		t.Fatalf("response = %v, want the one the transport made", got)
	}
}

func TestTheErrorIsHandedOnUntouched(t *testing.T) {
	sink := New()
	want := errors.New("no route to host")

	_, err := through(sink, nil, want)

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want the transport's own", err)
	}
}

// Nothing may wait on a listener that is not there, or one failing request
// would hold up every other one behind it.
func TestNobodyListeningDoesNotHoldUpTheRequest(t *testing.T) {
	sink := New()
	done := make(chan struct{})

	go func() {
		for i := range 100 {
			_, _ = through(sink, nil, fmt.Errorf("failure %d", i))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reporting held up the requests")
	}
}

func TestManyRequestsAtOnceAgreeOnOneAnswer(t *testing.T) {
	sink := New()
	var group sync.WaitGroup
	for range 50 {
		group.Go(func() {
			_, _ = through(sink, &http.Response{StatusCode: http.StatusOK}, nil)
		})
		group.Go(func() {
			_, _ = through(sink, nil, errors.New("connection refused"))
		})
	}
	group.Wait()

	// Which one landed last is a race; that it is one of the two and readable is
	// not.
	answering, reason := sink.State()
	if answering && reason != "" {
		t.Fatalf("state = (%v, %q), want the two to agree", answering, reason)
	}
}

// A sink nobody wired up stands in for a cluster spinoza has no client for.
func TestASinkThatIsNotThereIsHarmless(t *testing.T) {
	var sink *Sink

	sink.Saw(errors.New("connection refused"))

	answering, reason := sink.State()
	if !answering || reason != "" {
		t.Fatalf("state = (%v, %q), want the benefit of the doubt", answering, reason)
	}
	if sink.Changed() != nil {
		t.Fatal("a sink that is not there offered something to listen to")
	}
}

// The whole point of wrapping the transport: a real client, one real request,
// and the sink knows.
func TestARealRequestThroughARealClientIsSeen(t *testing.T) {
	sink := New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	client := &http.Client{Transport: sink.Wrap(http.DefaultTransport)}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()
	if answering, _ := sink.State(); !answering {
		t.Fatal("a request that worked left the cluster looking gone")
	}

	server.Close()
	_, gone := client.Get(server.URL)

	if gone == nil {
		t.Fatal("a request to a server that had gone came back fine")
	}
	answering, reason := sink.State()
	if answering {
		t.Fatal("a server that had gone was still counted as answering")
	}
	if reason == "" {
		t.Fatal("nothing was said about why")
	}
}
