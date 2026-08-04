package server

import (
	"crypto/rand"
	"fmt"
	"net"
	"net/url"
	"strings"
)

func NewToken() string {
	return rand.Text()
}

func BrowserURL(addr, token string) string {
	query := url.Values{AuthParam: []string{token}}
	return "http://" + addr + "/?" + query.Encode()
}

func CheckLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen address %q must be host:port", addr)
	}
	if host == "" {
		return fmt.Errorf("listen address %q binds every interface; spinoza holds your kubeconfig, so it only listens on loopback", addr)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return fmt.Errorf("listen address %q is not a loopback address", addr)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("listen address %q is not loopback; spinoza holds your kubeconfig and its origin guard would refuse remote browsers anyway", addr)
	}
	return nil
}

func IsBackendPath(path string) bool {
	if strings.HasPrefix(path, "/api/") {
		return true
	}
	if path == "/ws" {
		return true
	}
	return path == "/healthz"
}
