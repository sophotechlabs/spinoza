package server

import (
	"io/fs"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/broker"
)

type Server struct {
	broker broker.Broker
	assets fs.FS
}

func New(b broker.Broker, assets fs.FS) *Server {
	return &Server{broker: b, assets: assets}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/ws", s.handleWS)
	mux.Handle("/", http.FileServerFS(s.assets))
	return mux
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
