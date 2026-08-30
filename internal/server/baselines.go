package server

import (
	"net/http"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

// Baselines is where a past audit is kept so this one can say what is new.
type Baselines interface {
	Load(cluster string) (checks.Baseline, bool)
	Save(cluster string, taken checks.Baseline) error
	Clear(cluster string) error
}

// noBaselines is what a server runs with until it is given a store: taking a
// baseline succeeds and finds nothing to compare against, rather than failing.
type noBaselines struct{}

func (noBaselines) Load(string) (checks.Baseline, bool) {
	return checks.Baseline{}, false
}

func (noBaselines) Save(string, checks.Baseline) error {
	return nil
}

func (noBaselines) Clear(string) error {
	return nil
}

func (s *Server) UseBaselines(store Baselines) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseline = store
}

func (s *Server) baselines() Baselines {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseline
}

func (s *Server) takeBaseline(w http.ResponseWriter, r *http.Request) {
	taken := s.managerFor(r).CheckFingerprint(r.Context(), s.checkFilter(r))
	taken.TakenAt = s.now().UTC().Format(time.RFC3339)
	if err := s.baselines().Save(s.clusterKey(r), taken); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, api.Baseline{TakenAt: taken.TakenAt, Findings: len(taken.Keys), Checks: len(taken.Checks)})
}

func (s *Server) clearBaseline(w http.ResponseWriter, r *http.Request) {
	if err := s.baselines().Clear(s.clusterKey(r)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, api.Baseline{})
}
