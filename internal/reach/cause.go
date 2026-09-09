package reach

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strings"
	"syscall"
)

const (
	Timeout      = "timeout"
	Refused      = "refused"
	Untrusted    = "tls"
	Unresolved   = "dns"
	Unauthorized = "unauthorized"
	Unknown      = "other"
)

type marker struct {
	cause string
	texts []string
}

var markers = []marker{
	{Timeout, []string{"deadline exceeded", "timed out", "i/o timeout", "handshake timeout", "client.timeout", "request timeout", "timeout awaiting"}},
	{Refused, []string{"connection refused", "no route to host", "network is unreachable", "connection reset"}},
	{Untrusted, []string{"x509:", "tls: failed to verify certificate", "certificate signed by unknown authority", "certificate is not trusted", "tls: "}},
	{Unresolved, []string{"no such host", "server misbehaving", "dns error"}},
	{Unauthorized, []string{"unauthorized", "is forbidden", "getting credentials", "could not get credentials", "invalid bearer token"}},
}

func Cause(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Timeout
	}
	if timely, ok := errors.AsType[net.Error](err); ok && timely.Timeout() {
		return Timeout
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return Refused
	}
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return Untrusted
	}
	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return Untrusted
	}
	if _, ok := errors.AsType[x509.HostnameError](err); ok {
		return Untrusted
	}
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return Unresolved
	}
	return CauseOf(err.Error())
}

func CauseOf(reason string) string {
	if reason == "" {
		return ""
	}
	lowered := strings.ToLower(reason)
	for _, one := range markers {
		for _, text := range one.texts {
			if strings.Contains(lowered, text) {
				return one.cause
			}
		}
	}
	return Unknown
}
