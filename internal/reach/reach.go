// Package reach keeps what the requests spinoza is already making say about
// whether the cluster is answering. Every client is built on one transport, so
// one wrapper sees every list, watch, read and write: a request that came back
// with nothing is the plainest evidence there is that the cluster is gone, and
// a reply of any kind — even a refusal — is evidence the other way.
package reach

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

// Sink holds the last thing the transport saw and tells whoever is listening
// when that changes. Nothing is ever sent for an answer that repeats the one
// before it.
type Sink struct {
	mu        sync.Mutex
	answering bool
	reason    string
	changed   chan struct{}
}

// New starts out assuming the cluster answers: spinoza has asked it nothing yet,
// and a window should not be told the worst on no evidence.
func New() *Sink {
	return &Sink{answering: true, changed: make(chan struct{}, 1)}
}

// Wrap is the transport wrapper. It reports what each request came back with
// and hands the response on untouched.
func (s *Sink) Wrap(next http.RoundTripper) http.RoundTripper {
	return &watched{next: next, sink: s}
}

// Saw records what one request came back with. A request the caller gave up on
// says nothing about the cluster: a closed window cancels everything it had
// open, and that is not an outage.
func (s *Sink) Saw(err error) {
	if s == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	if err == nil {
		s.record(true, "")
		return
	}
	s.record(false, err.Error())
}

// Changed fires once for every change of mind. A sink nobody wired up never
// fires, which in a select is the same as not being there.
func (s *Sink) Changed() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.changed
}

func (s *Sink) State() (bool, string) {
	if s == nil {
		return true, ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.answering, s.reason
}

func (s *Sink) record(answering bool, reason string) {
	s.mu.Lock()
	changed := s.answering != answering
	if s.reason != reason {
		changed = true
	}
	s.answering = answering
	s.reason = reason
	s.mu.Unlock()
	if !changed {
		return
	}
	s.tell()
}

// tell never waits. One pending word that the answer changed is enough, and a
// request must not be held up to deliver it.
func (s *Sink) tell() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

type watched struct {
	next http.RoundTripper
	sink *Sink
}

func (w *watched) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := w.next.RoundTrip(req)
	w.sink.Saw(err)
	return resp, err
}
