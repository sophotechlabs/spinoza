package reach_test

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/reach"
)

func TestEachKindOfFailureIsNamedByItsCause(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		cause string
	}{
		{"a deadline", context.DeadlineExceeded, reach.Timeout},
		{"a wrapped deadline", fmt.Errorf("asking the apiserver: %w", context.DeadlineExceeded), reach.Timeout},
		{"a refused connection", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, reach.Refused},
		{"an unknown authority", x509.UnknownAuthorityError{}, reach.Untrusted},
		{"a name that does not resolve", &net.DNSError{Err: "no such host", IsNotFound: true}, reach.Unresolved},
		{"nothing at all", nil, ""},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			if got := reach.Cause(one.err); got != one.cause {
				t.Fatalf("Cause(%v) = %q, want %q", one.err, got, one.cause)
			}
		})
	}
}

func TestTheTextOfAFailureIsEnoughToNameItsCause(t *testing.T) {
	cases := []struct {
		reason string
		cause  string
	}{
		{`Get "https://10.0.0.1:6443/version": context deadline exceeded`, reach.Timeout},
		{"net/http: TLS handshake timeout", reach.Timeout},
		{`Get "https://10.0.0.1:6443/version": dial tcp 10.0.0.1:6443: connect: connection refused`, reach.Refused},
		{"tls: failed to verify certificate: x509: certificate signed by unknown authority", reach.Untrusted},
		{`dial tcp: lookup api.example.com: no such host`, reach.Unresolved},
		{`Unauthorized`, reach.Unauthorized},
		{"the cluster said something nobody has classified", reach.Unknown},
		{"", ""},
	}

	for _, one := range cases {
		t.Run(one.reason, func(t *testing.T) {
			if got := reach.CauseOf(one.reason); got != one.cause {
				t.Fatalf("CauseOf(%q) = %q, want %q", one.reason, got, one.cause)
			}
		})
	}
}

func TestAHandshakeThatTimedOutIsATimeoutRatherThanACertificateProblem(t *testing.T) {
	handshake := reach.CauseOf("net/http: TLS handshake timeout")
	deadline := reach.CauseOf(`Get "https://127.0.0.1:6443/version": context deadline exceeded`)
	if handshake != deadline {
		t.Fatalf("a handshake timeout is %q and a deadline is %q; one paused apiserver must not read as two faults", handshake, deadline)
	}
}

func TestACancelledRequestIsNotReportedAsACertificateProblem(t *testing.T) {
	if got := reach.Cause(errors.New("client rate limiter Wait returned an error: context canceled")); got != reach.Unknown {
		t.Fatalf("Cause = %q, want %q", got, reach.Unknown)
	}
}
