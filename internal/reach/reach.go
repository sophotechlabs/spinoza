package reach

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

type Sink struct {
	mu        sync.Mutex
	answering bool
	reason    string
	changed   chan struct{}
}

func New() *Sink {
	return &Sink{answering: true, changed: make(chan struct{}, 1)}
}

func (s *Sink) Wrap(next http.RoundTripper) http.RoundTripper {
	return &watched{next: next, sink: s}
}

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
