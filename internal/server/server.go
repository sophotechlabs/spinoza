package server

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/resources"
)

type Server struct {
	mgr    *resources.Manager
	assets fs.FS
}

func New(mgr *resources.Manager, assets fs.FS) *Server {
	return &Server{mgr: mgr, assets: assets}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/api/resources", s.handleResources)
	mux.HandleFunc("/ws", s.handleWS)
	mux.Handle("/", http.FileServerFS(s.assets))
	return mux
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.mgr.Resources())
}
