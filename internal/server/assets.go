package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		s.serveIndex(w, r)
		return
	}
	s.files.ServeHTTP(w, r)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	doc, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "index.html is missing from the bundled assets")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(InjectHead(doc, s.headFor(r, ViewBrowser)))
}

func (s *Server) headFor(r *http.Request, view string) string {
	if s.inCluster() && !signedIn(r) {
		return ViewScript(view)
	}
	return s.indexHead(r, view)
}

func signedIn(r *http.Request) bool {
	_, ok := auth.IdentityFrom(r.Context())
	return ok
}

func (s *Server) IndexHead(view string) string {
	return s.indexHead(nil, view)
}

func (s *Server) indexHead(r *http.Request, view string) string {
	values := servedSettings(s.stored().All())
	if r != nil && s.inCluster() {
		values = s.settingsFor(r)
	}
	return TokenScript(s.token) + SettingsScript(values) + ViewScript(view) +
		StartScript(s.start.view, s.start.context)
}

type startRoute struct {
	view    string
	context string
}

func (s *Server) StartOn(view, context string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.start = startRoute{view: view, context: context}
}

func TokenScript(token string) string {
	return "<script>window.__SPINOZA_TOKEN__=" + scriptValue(token) + ";</script>"
}

func scriptValue(raw string) string {
	return strings.ReplaceAll(strconv.Quote(raw), "</", `<\/`)
}

func InjectHead(doc []byte, markup string) []byte {
	closing := []byte("</head>")
	return bytes.Replace(doc, closing, append([]byte(markup), closing...), 1)
}
