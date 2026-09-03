package main

import (
	"net/http"
	"testing"
)

func TestHTTPServerHasFiniteHeaderAndIdleDeadlines(t *testing.T) {
	srv := configuredHTTPServer("127.0.0.1:0", http.NotFoundHandler())
	if srv.ReadHeaderTimeout != httpHeaderTimeout || srv.ReadHeaderTimeout <= 0 {
		t.Fatalf("header timeout = %s", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout != httpIdleTimeout || srv.IdleTimeout <= 0 {
		t.Fatalf("idle timeout = %s", srv.IdleTimeout)
	}
}
