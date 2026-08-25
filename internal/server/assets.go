package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"strconv"
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
	return TokenScript(s.token) + SettingsScript(s.stored().All()) + ViewScript(view)
}

func TokenScript(token string) string {
	return "<script>window.__SPINOZA_TOKEN__=" + strconv.Quote(token) + ";</script>"
}

func InjectHead(doc []byte, markup string) []byte {
	closing := []byte("</head>")
	return bytes.Replace(doc, closing, append([]byte(markup), closing...), 1)
}
