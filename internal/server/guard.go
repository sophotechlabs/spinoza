package server

import (
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/sophotechlabs/spinoza/internal/safe"
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
			slog.Warn(
				"refused a request that did not look local",
				"path", strconv.Quote(r.URL.Path),
				"host", strconv.Quote(r.Host),
				"origin", strconv.Quote(r.Header.Get("Origin")),
				"fetchSite", strconv.Quote(r.Header.Get("Sec-Fetch-Site")),
			)
			writeError(w, http.StatusForbidden, "spinoza answers local requests only")
			return
		}
		if !publicAsset(r) && !s.authorize(w, r) {
			slog.Warn(
				"refused a request without this run's token",
				"path", strconv.Quote(r.URL.Path),
				"origin", strconv.Quote(r.Header.Get("Origin")),
			)
			writeError(w, http.StatusUnauthorized, "spinoza needs the token it printed at startup")
			return
		}
		if upgrading(r) {
			defer safe.Recover("the socket on " + r.URL.Path)
			handler(w, r)
			return
		}
		recorded := &recorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		defer func() {
			finish(recorded, r, started, recover())
		}()
		handler(recorded, r)
	}
}

func finish(recorded *recorder, r *http.Request, started time.Time, caught any) {
	if caught != nil {
		safe.Log("the handler for "+r.URL.Path, caught)
		if !recorded.wrote {
			writeError(recorded, http.StatusInternalServerError, "spinoza broke handling that request; the terminal has the details")
		}
	}
	if !mutating(r.Method) {
		return
	}
	slog.Info(
		"acted on the cluster",
		"method", strconv.Quote(r.Method),
		"path", strconv.Quote(r.URL.Path),
		"query", strconv.Quote(loggableQuery(r)),
		"status", recorded.status,
		"took", time.Since(started).Round(time.Millisecond),
	)
}

func upgrading(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func mutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func loggableQuery(r *http.Request) string {
	values := r.URL.Query()
	values.Del(AuthParam)
	return values.Encode()
}

type recorder struct {
	http.ResponseWriter

	status int
	wrote  bool
}

func (rec *recorder) WriteHeader(status int) {
	rec.status = status
	rec.wrote = true
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *recorder) Write(p []byte) (int, error) {
	rec.wrote = true
	return rec.ResponseWriter.Write(p)
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

func publicAsset(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if r.URL.Path == "/favicon.svg" {
		return true
	}
	if r.URL.Path == "/assets/" {
		return false
	}
	return strings.HasPrefix(r.URL.Path, "/assets/")
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
	origin := r.Header.Get("Origin")
	if origin == "" {
		if topLevelNavigation(r) {
			return true
		}
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	return allowedOrigin(origin, r.Host)
}

func topLevelNavigation(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Mode") != "navigate" {
		return false
	}
	return r.Header.Get("Sec-Fetch-Dest") == "document"
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
