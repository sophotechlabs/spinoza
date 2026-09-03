package main

import (
	"net/http"
	"time"
)

const httpHeaderTimeout = 10 * time.Second

const httpIdleTimeout = 60 * time.Second

func configuredHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: httpHeaderTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
}
