package server

import (
	"io"
	"net/http"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/baseline"
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

const maxBaselineBytes = 64 << 20

// saveBaselineFile hands the baseline out as the file it is kept in, so it can
// live in a repository or be given to somebody else.
func (s *Server) saveBaselineFile(w http.ResponseWriter, r *http.Request) {
	taken, ok := s.baselines().Load(s.clusterKey(r))
	if !ok {
		writeError(w, http.StatusNotFound, "no baseline has been taken on this cluster")
		return
	}
	body, err := baseline.Encode(taken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="spinoza-baseline.json"`)
	_, _ = w.Write(body)
}

// loadBaselineFile takes one back. What it names as its own cluster is kept, so
// a baseline carried from somewhere else says where it came from.
func (s *Server) loadBaselineFile(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBaselineBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "the baseline could not be read")
		return
	}
	taken, decodeErr := baseline.Decode(body)
	if decodeErr != nil {
		writeError(w, http.StatusBadRequest, decodeErr.Error())
		return
	}
	if saveErr := s.baselines().Save(s.clusterKey(r), taken); saveErr != nil {
		writeError(w, http.StatusInternalServerError, saveErr.Error())
		return
	}
	writeJSON(w, api.Baseline{
		TakenAt:  taken.TakenAt,
		Cluster:  taken.Cluster,
		Findings: len(taken.Keys),
		Checks:   len(taken.Checks),
	})
}

func (s *Server) takeBaseline(w http.ResponseWriter, r *http.Request) {
	taken := s.managerFor(r).CheckFingerprint(r.Context(), s.checkFilter(r))
	taken.TakenAt = s.now().UTC().Format(time.RFC3339)
	taken.Cluster = s.clusterKey(r)
	if err := s.baselines().Save(s.clusterKey(r), taken); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, api.Baseline{
		TakenAt:  taken.TakenAt,
		Cluster:  taken.Cluster,
		Findings: len(taken.Keys),
		Checks:   len(taken.Checks),
	})
}

func (s *Server) clearBaseline(w http.ResponseWriter, r *http.Request) {
	if err := s.baselines().Clear(s.clusterKey(r)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, api.Baseline{})
}
