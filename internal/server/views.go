package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/issues"
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
	return s.checkFilterOn(r, s.clusterKey(r))
}

func (s *Server) checkFilterOn(r *http.Request, cluster string) checks.Filter {
	keep := checks.ParseFilter(r.URL.Query())
	held := s.stored().All()
	keep.Rules = checks.ParseRules(held[checks.RulesKey])
	keep.Silencers = checks.Silencers(keep.Rules)
	keep.Mutes = checks.ParseMutes(held[checks.MutesKey], cluster)
	if taken, ok := s.baselines().Load(cluster); ok {
		if taken.Cluster == cluster {
			taken.Cluster = ""
		}
		keep.Base = &taken
	}
	return keep
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

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	graph := s.managerFor(r).TrafficGraph(r.Context())
	if nothingButAnError(graph) {
		writeJSONStatus(w, http.StatusBadGateway, graph)
		return
	}
	writeJSON(w, graph)
}

func nothingButAnError(graph api.TrafficGraph) bool {
	if graph.Error == "" {
		return false
	}
	if len(graph.Nodes) > 0 {
		return false
	}
	return len(graph.Edges) == 0
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
	on := s.clusterKey(r)
	for at := range queue.Rows {
		queue.Rows[at].Cluster = on
	}
	writeJSON(w, pagedQueue(queue, r))
}

func pagedQueue(queue api.IssueQueue, r *http.Request) api.IssueQueue {
	query := r.URL.Query()
	order := issues.OrderOf(query.Get("sort"))
	issues.Rank(queue.Rows, order)
	queue.Tally = issues.Tally(queue.Rows)
	rows, next := issues.Page(
		queue.Rows,
		issues.DecodeCursor(query.Get("after")),
		issues.PageSize(shownOf(query.Get("shown"))),
		order,
	)
	queue.Rows = rows
	queue.Next = next
	return queue
}

func shownOf(raw string) int {
	asked, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return asked
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
