package server

import (
	"net/http"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/topology"
)

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Graph(r.Context()))
}

func (s *Server) handleCheckPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	id := query.Get("check")
	if id == "" {
		writeError(w, http.StatusBadRequest, "check is required")
		return
	}
	page, err := s.managerFor(r).CheckPage(r.Context(), id, query.Get("after"), s.checkFilter(r))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, page)
}

func (s *Server) handleChecks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Checks(r.Context(), s.checkFilter(r)))
}

func (s *Server) checkFilter(r *http.Request) checks.Filter {
	keep := checks.ParseFilter(r.URL.Query())
	held := s.stored().All()
	keep.Rules = checks.ParseRules(held[checks.RulesKey])
	keep.Mutes = checks.ParseMutes(held[checks.MutesKey], s.clusterKey(r))
	if taken, ok := s.baselines().Load(s.clusterKey(r)); ok {
		keep.Base = &taken
	}
	return keep
}

// clusterKey is what mutes and baselines are filed under. A request that named
// no cluster still means one, so the resolved id is used rather than the empty
// string a single-cluster window sends.
func (s *Server) clusterKey(r *http.Request) string {
	_, on := s.lookup(clusterOf(r))
	return on
}

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Topology(r.Context(), topologyRequest(r)))
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
	writeJSON(w, s.managerFor(r).Flux(r.Context()))
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
	history, historyErr := s.managerFor(r).MetricHistory(r.Context(), namespace, pod, span)
	if historyErr != nil {
		writeAPIError(w, historyErr)
		return
	}
	writeJSON(w, history)
}

func (s *Server) handleTrafficSupport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).TrafficSupport(r.Context()))
}

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).TrafficGraph(r.Context()))
}

func (s *Server) handleFluxOverview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).FluxOverview(r.Context()))
}

func (s *Server) handleArgo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Argo(r.Context()))
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Overview(r.Context()))
}

func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	queue := s.managerFor(r).Issues(r.Context())
	on := s.clusterOf(r)
	for at := range queue.Rows {
		queue.Rows[at].Cluster = on
	}
	writeJSON(w, queue)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.managerFor(r).Metrics(r.Context()))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	events, err := s.managerFor(r).Events(r.Context(), query.Get("namespace"), query.Get("uid"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, events)
}
