package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		s.serveIndex(w)
		return
	}
	s.files.ServeHTTP(w, r)
}

func (s *Server) serveIndex(w http.ResponseWriter) {
	doc, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "index.html is missing from the bundled assets")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(InjectHead(doc, s.IndexHead(ViewBrowser)))
}

func (s *Server) IndexHead(view string) string {
	return TokenScript(s.token) + SettingsScript(s.stored().All()) + ViewScript(view) +
		StartScript(s.start.view, s.start.context)
}

type startRoute struct {
	view    string
	context string
}

// StartOn is what a run with no address bar and nobody to click uses to open
// somewhere other than the default.
func (s *Server) StartOn(view, context string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.start = startRoute{view: view, context: context}
}

func TokenScript(token string) string {
	return "<script>window.__SPINOZA_TOKEN__=" + scriptValue(token) + ";</script>"
}

// strconv.Quote escapes a Go string, not an HTML one: a value holding </script>
// would close the tag it sits in. Inside a JS string <\/ reads as / and cannot.
func scriptValue(raw string) string {
	return strings.ReplaceAll(strconv.Quote(raw), "</", `<\/`)
}

func InjectHead(doc []byte, markup string) []byte {
	closing := []byte("</head>")
	return bytes.Replace(doc, closing, append([]byte(markup), closing...), 1)
}
