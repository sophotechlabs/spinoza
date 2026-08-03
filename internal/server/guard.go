package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
)

const desktopScheme = "wails"

const desktopHost = "wails.localhost"

const readLimit = 64 << 10

func accept(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(readLimit)
	return conn, nil
}

func guard(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocal(r) {
			writeError(w, http.StatusForbidden, "spinoza answers local requests only")
			return
		}
		handler(w, r)
	}
}

func isLocal(r *http.Request) bool {
	if !loopbackAuthority(r.Host) {
		return false
	}
	return allowedOrigin(r.Header.Get("Origin"))
}

func loopbackAuthority(authority string) bool {
	if authority == "" {
		return false
	}
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		host = authority
	}
	if host == "localhost" {
		return true
	}
	if host == desktopHost {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func allowedOrigin(origin string) bool {
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme == desktopScheme {
		return true
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return loopbackAuthority(parsed.Host)
}
