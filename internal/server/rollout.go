package server

import (
	"context"
	"net/http"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/compare"
	"github.com/sophotechlabs/spinoza/internal/rollout"
)

func (s *Server) rolloutRevisions(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	found, err := rollout.List(r.Context(), rolloutReads{backend: s.managerFor(r)}, ref)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, found)
}

func (s *Server) rolloutDiff(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	query := r.URL.Query()
	from, ok := rolloutRevisionOf(w, query.Get("from"), "from")
	if !ok {
		return
	}
	to, ok := rolloutRevisionOf(w, query.Get("to"), "to")
	if !ok {
		return
	}
	diff, err := rollout.Diff(r.Context(), rolloutReads{backend: s.managerFor(r)}, ref, from, to)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, diff)
}

func rolloutRevisionOf(w http.ResponseWriter, raw, name string) (int64, bool) {
	number, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, name+" must be a revision number")
		return 0, false
	}
	return number, true
}

type rolloutReads struct {
	backend Reader
}

func (reads rolloutReads) Get(ctx context.Context, ref api.ObjectRef) (*unstructured.Unstructured, error) {
	detail, err := reads.backend.Object(ctx, ref)
	if err != nil {
		return nil, err
	}
	return compare.Parse(detail.YAML)
}

func (reads rolloutReads) List(ctx context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error) {
	return reads.backend.ListKind(ctx, ref)
}
