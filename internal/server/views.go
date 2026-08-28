package server

import (
	"net/http"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/topology"
)

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Graph(r.Context()))
}

func (s *Server) handleCheckPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	id := query.Get("check")
	if id == "" {
		writeError(w, http.StatusBadRequest, "check is required")
		return
	}
	page, err := s.manager().CheckPage(r.Context(), id, query.Get("after"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, page)
}

func (s *Server) handleChecks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Checks(r.Context()))
}

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Topology(r.Context(), topologyRequest(r)))
}

func topologyRequest(r *http.Request) topology.Request {
	query := r.URL.Query()
	return topology.Request{
		Namespace: query.Get("namespace"),
		Root: api.ObjectRef{
			Group:     query.Get("rootGroup"),
			Version:   query.Get("rootVersion"),
			Resource:  query.Get("rootResource"),
			Namespace: query.Get("rootNamespace"),
			Name:      query.Get("rootName"),
		},
		Expanded: expandedIDs(query.Get("expand")),
	}
}

func expandedIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func (s *Server) handleFlux(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Flux(r.Context()))
}

func (s *Server) handleMetricHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	namespace := query.Get("namespace")
	pod := query.Get("pod")
	if namespace == "" || pod == "" {
		writeError(w, http.StatusBadRequest, "namespace and pod are required")
		return
	}
	span, err := prom.ParseSpan(query.Get("range"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	history, historyErr := s.manager().MetricHistory(r.Context(), namespace, pod, span)
	if historyErr != nil {
		writeAPIError(w, historyErr)
		return
	}
	writeJSON(w, history)
}

func (s *Server) handleTrafficSupport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().TrafficSupport(r.Context()))
}

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().TrafficGraph(r.Context()))
}

func (s *Server) handleFluxOverview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().FluxOverview(r.Context()))
}

func (s *Server) handleArgo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Argo(r.Context()))
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Overview(r.Context()))
}

func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Issues(r.Context()))
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.manager().Metrics(r.Context()))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	events, err := s.manager().Events(r.Context(), query.Get("namespace"), query.Get("uid"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, events)
}
