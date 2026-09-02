package portforward

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"k8s.io/client-go/tools/portforward"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

type countingDialer struct {
	called bool
}

func (dialer *countingDialer) Dial(...string) (streamhttp.Connection, string, error) {
	dialer.called = true
	return nil, "", errors.New("dial should not be reached")
}

func TestRunRejectsInvalidPortsBeforeDialing(t *testing.T) {
	cases := []struct {
		name       string
		localPort  int32
		remotePort int32
	}{
		{name: "negative local port", localPort: -1, remotePort: 8080},
		{name: "negative remote port", localPort: 0, remotePort: -1},
		{name: "oversized remote port", localPort: 0, remotePort: 65536},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dialer := &countingDialer{}
			runner := runnerWith(dialer, nil)

			err := runner.Run(
				t.Context(),
				"flux-system",
				"web",
				tc.localPort,
				tc.remotePort,
				make(chan int32, 1),
				make(chan struct{}),
			)

			if err == nil {
				t.Fatal("an invalid port pair started a forward")
			}
			if dialer.called {
				t.Fatal("the apiserver was dialed before the ports were validated")
			}
		})
	}
}

func TestAnnounceDoesNotBlockWhenNobodyReceivesThePort(t *testing.T) {
	stop := make(chan struct{})
	forwarderReady := make(chan struct{})
	forwarder, err := portforward.NewForStreaming(
		&fakeDialer{conn: &fakeConnection{closed: make(chan bool)}},
		[]string{"0:8080"},
		stop,
		forwarderReady,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("new forwarder: %v", err)
	}
	forwarded := make(chan error, 1)
	go func() {
		forwarded <- forwarder.ForwardPorts()
	}()

	select {
	case <-forwarderReady:
	case err := <-forwarded:
		t.Fatalf("forward ended before becoming ready: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("forward never became ready")
	}
	announced := make(chan struct{})
	go func() {
		announce(context.Background(), forwarder, forwarderReady, make(chan struct{}), make(chan int32))
		close(announced)
	}()
	select {
	case <-announced:
	case <-time.After(10 * time.Second):
		t.Fatal("an abandoned readiness receiver blocked the forward")
	}

	close(stop)
	select {
	case err := <-forwarded:
		if err != nil {
			t.Fatalf("forward returned %v after stop", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("forward did not stop")
	}
}

func TestRelayStopLeavesTheForwardAloneAfterItAlreadyEnded(t *testing.T) {
	stop := make(chan struct{})
	done := make(chan struct{})
	halt := make(chan struct{})
	close(done)
	returned := make(chan struct{})
	go func() {
		relayStop(context.Background(), stop, done, halt)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(10 * time.Second):
		t.Fatal("the stop relay stayed behind after the forward ended")
	}
	select {
	case <-halt:
		t.Fatal("an already-finished forward was halted again")
	default:
	}
}
