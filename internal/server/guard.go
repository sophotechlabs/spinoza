package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
)

const desktopScheme = "wails"

const desktopHost = "wails.localhost"

const readLimit = 64 << 10

const AuthHeader = "X-Spinoza-Token"

const AuthParam = "token"

const authCookie = "spinoza_token"

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

func (s *Server) guard(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocal(r) {
			writeError(w, http.StatusForbidden, "spinoza answers local requests only")
			return
		}
		if !s.authorize(w, r) {
			writeError(w, http.StatusUnauthorized, "spinoza needs the token it printed at startup")
			return
		}
		handler(w, r)
	}
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) bool {
	presented, fromQuery := presentedToken(r)
	if !tokenMatches(s.token, presented) {
		return false
	}
	if fromQuery {
		http.SetCookie(w, &http.Cookie{
			Name:     authCookie,
			Value:    presented,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
	}
	return true
}

func presentedToken(r *http.Request) (string, bool) {
	header := r.Header.Get(AuthHeader)
	if header != "" {
		return header, false
	}
	query := r.URL.Query().Get(AuthParam)
	if query != "" {
		return query, true
	}
	cookie, err := r.Cookie(authCookie)
	if err != nil {
		return "", false
	}
	return cookie.Value, false
}

func tokenMatches(want, presented string) bool {
	if want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(presented)) == 1
}

func isLocal(r *http.Request) bool {
	if !loopbackAuthority(r.Host) {
		return false
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	return allowedOrigin(r.Header.Get("Origin"), r.Host)
}

func loopbackAuthority(authority string) bool {
	if authority == "" {
		return false
	}
	host := hostOf(authority)
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

func hostOf(authority string) string {
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		return authority
	}
	return host
}

func desktopAuthority(authority string) bool {
	host := hostOf(authority)
	if host == desktopScheme {
		return true
	}
	return host == desktopHost
}

func allowedOrigin(origin, authority string) bool {
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme == desktopScheme {
		return desktopAuthority(parsed.Host)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if desktopAuthority(parsed.Host) {
		return true
	}
	return parsed.Host == authority
}
