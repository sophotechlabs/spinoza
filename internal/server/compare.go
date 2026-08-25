package server

import (
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/compare"
)

func (s *Server) compare(w http.ResponseWriter, r *http.Request, ref api.ObjectRef) {
	query := r.URL.Query()
	against := api.ContextRef{Kubeconfig: query.Get("againstKubeconfig"), Name: query.Get("against")}
	if against.Name == "" {
		writeError(w, http.StatusBadRequest, "name the context to compare against")
		return
	}
	keep := query.Get("raw") == queryTrue
	here, err := s.manager().Object(r.Context(), ref)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	left, leftErr := compare.Rendered(here.YAML, keep)
	if leftErr != nil {
		writeAPIError(w, leftErr)
		return
	}
	writeJSON(w, s.against(r, ref, against, left, keep))
}

func (s *Server) against(r *http.Request, ref api.ObjectRef, against api.ContextRef, left string, keep bool) api.Comparison {
	result := api.Comparison{
		Left:         left,
		LeftContext:  s.cluster.Contexts().Current.Name,
		RightContext: against.Name,
	}
	raw, err := s.cluster.Read(r.Context(), against, farSide(r, ref))
	if err != nil {
		result.Missing = missingReason(err)
		return result
	}
	right, renderErr := compare.Rendered(raw, keep)
	if renderErr != nil {
		result.Missing = renderErr.Error()
		return result
	}
	result.Right = right
	result.Identical = left == right
	return result
}

func (s *Server) compareKind(w http.ResponseWriter, r *http.Request) {
	ref := refFrom(r)
	if ref.Version == "" || ref.Resource == "" {
		writeError(w, http.StatusBadRequest, "version and resource are required")
		return
	}
	query := r.URL.Query()
	against := api.ContextRef{Kubeconfig: query.Get("againstKubeconfig"), Name: query.Get("against")}
	if against.Name == "" {
		writeError(w, http.StatusBadRequest, "name the context to compare against")
		return
	}
	here, err := s.manager().ListKind(r.Context(), ref)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	far := farSide(r, ref)
	there, listErr := s.cluster.List(r.Context(), against, far)
	if listErr != nil {
		writeAPIError(w, listErr)
		return
	}
	byName := ref.Namespace != "" && far.Namespace != ref.Namespace
	objects := compare.Kinds(here, there, byName)
	same, differs, onlyHere, onlyThere := compare.Tally(objects)
	writeJSON(w, api.KindComparison{
		Resource:      ref.Resource,
		LeftContext:   s.cluster.Contexts().Current.Name,
		RightContext:  against.Name,
		Namespace:     ref.Namespace,
		Objects:       objects,
		Same:          same,
		Differs:       differs,
		OnlyHere:      onlyHere,
		OnlyThere:     onlyThere,
		MatchedByName: byName,
	})
}

func farSide(r *http.Request, ref api.ObjectRef) api.ObjectRef {
	query := r.URL.Query()
	far := ref
	namespace := query.Get("againstNamespace")
	if namespace != "" {
		far.Namespace = namespace
	}
	name := query.Get("againstName")
	if name != "" {
		far.Name = name
	}
	return far
}

func missingReason(err error) string {
	if apierrors.IsNotFound(err) {
		return "that context has no such object"
	}
	return err.Error()
}
