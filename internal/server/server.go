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
	mux.HandleFunc("/api/resources", cors(s.handleResources))
	mux.HandleFunc("/api/gitops/graph", cors(s.handleGraph))
	mux.HandleFunc("/api/flux", cors(s.handleFlux))
	mux.HandleFunc("/api/metrics", cors(s.handleMetrics))
	mux.HandleFunc("/ws", s.handleWS)
	mux.Handle("/", http.FileServerFS(s.assets))
	return mux
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.mgr.Resources())
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.mgr.Graph(r.Context()))
}

func (s *Server) handleFlux(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.mgr.Flux(r.Context()))
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.mgr.Metrics(r.Context()))
}
