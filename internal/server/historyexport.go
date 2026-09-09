package server

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	exportPageSize = 1000
	exportPages    = 1000
	exportRowCap   = exportPageSize * exportPages
)

var auditColumns = []string{
	"id", "source", "cluster", "at", "verb", "actor", "group", "version",
	"resource", "kind", "namespace", "name", "detail", "was", "outcome", "message",
}

type auditFilter struct {
	source string
	limit  int
	from   cursors
	fleet  bool
}

func (s *Server) exportHistory(w http.ResponseWriter, r *http.Request) {
	past := s.recorder()
	if past == nil {
		writeError(w, http.StatusServiceUnavailable, api.HistoryOff)
		return
	}
	asked, err := auditFilterOf(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.URL.Query().Get("format") == "json" {
		s.exportHistoryJSON(w, r, asked)
		return
	}
	s.exportHistoryCSV(w, r, asked)
}

func (s *Server) exportHistoryCSV(w http.ResponseWriter, r *http.Request, asked auditFilter) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="spinoza-history.csv"`)
	out := csv.NewWriter(w)
	_ = out.Write(auditColumns)
	written := 0
	s.eachHistoryPage(w, r, asked, func(entries []api.HistoryEntry) {
		for _, one := range entries {
			_ = out.Write(spreadsheetCells(auditRow(one)))
		}
		written += len(entries)
		out.Flush()
	})
	out.Flush()
}

func (s *Server) exportHistoryJSON(w http.ResponseWriter, r *http.Request, asked auditFilter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="spinoza-history.json"`)
	first := true
	_, _ = w.Write([]byte("["))
	s.eachHistoryPage(w, r, asked, func(entries []api.HistoryEntry) {
		for _, one := range entries {
			if !first {
				_, _ = w.Write([]byte(","))
			}
			first = false
			body, err := json.Marshal(one)
			if err != nil {
				continue
			}
			_, _ = w.Write(body)
		}
	})
	_, _ = w.Write([]byte("]"))
}

func (s *Server) eachHistoryPage(
	w http.ResponseWriter, r *http.Request, asked auditFilter, hand func([]api.HistoryEntry),
) {
	from := asked.from
	most := exportRowCap
	if asked.limit > 0 {
		most = asked.limit
	}
	written := 0
	for range exportPages {
		found, ok := s.historyFrom(w, r, asked.source, pageSizeFor(most, written), from, asked.fleet)
		if !ok {
			return
		}
		if len(found.Entries) == 0 {
			return
		}
		entries := found.Entries
		if written+len(entries) > most {
			entries = entries[:most-written]
		}
		hand(entries)
		written += len(entries)
		if !found.More || written >= most {
			return
		}
		next := cursors{changes: found.Next, actions: found.NextAction}
		if next == from {
			return
		}
		from = next
	}
}

func pageSizeFor(most, written int) int {
	room := most - written
	if room < exportPageSize {
		return room
	}
	return exportPageSize
}

func auditFilterOf(r *http.Request) (auditFilter, error) {
	asked := auditFilter{
		source: r.URL.Query().Get("source"),
		fleet:  r.URL.Query().Get("fleet") == queryTrue,
	}
	if asked.source == "" {
		asked.source = api.HistoryAll
	}
	if asked.source != api.HistoryAll && asked.source != api.HistoryAction && asked.source != api.HistoryChange {
		return auditFilter{}, errBadSource
	}
	limit, err := historyLimit(r)
	if err != nil {
		return auditFilter{}, err
	}
	asked.limit = limit
	after, afterErr := historyAfter(r)
	if afterErr != nil {
		return auditFilter{}, afterErr
	}
	afterAction, actionErr := historyAfterAction(r)
	if actionErr != nil {
		return auditFilter{}, actionErr
	}
	asked.from = cursors{changes: after, actions: afterAction}
	return asked, nil
}

func auditRow(one api.HistoryEntry) []string {
	return []string{
		strconv.FormatInt(one.ID, 10), one.Source, one.Cluster, one.At,
		one.Verb, one.Actor, one.Group, one.Version, one.Resource, one.Kind,
		one.Namespace, one.Name, one.Detail, one.Was, one.Outcome, one.Message,
	}
}
