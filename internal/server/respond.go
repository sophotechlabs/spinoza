package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/gitops"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func writeJSON(w http.ResponseWriter, payload any) {
	writeJSONStatus(w, http.StatusOK, payload)
}

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(payload)
	if err != nil {
		slog.Warn("a response could not be encoded", "error", err)
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, into any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	if err := decoder.Decode(into); err != nil {
		return err
	}
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("request body contains more than one json value")
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	err := json.NewEncoder(w).Encode(api.Failure{Message: message})
	if err != nil {
		slog.Warn("an error response could not be encoded", "error", err)
	}
}

func cannotReachCluster(err error) bool {
	if errors.Is(err, prom.ErrUnavailable) {
		return true
	}
	return errors.Is(err, resources.ErrNotSynced)
}

func unreachable(err error) bool {
	switch {
	case apierrors.IsServiceUnavailable(err), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return true
	case apierrors.IsInternalError(err), apierrors.IsTooManyRequests(err):
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func writeAPIError(w http.ResponseWriter, err error) {
	writeError(w, statusFor(err), err.Error())
}

func oversized(err error) bool {
	var tooBig *http.MaxBytesError
	return errors.As(err, &tooBig)
}

func askedForSomethingWrong(err error) bool {
	for _, sentinel := range []error{
		checks.ErrNoSuchCheck,
		inspect.ErrInvalidUID,
		inspect.ErrNoResourceVersion,
		gitops.ErrNotAnApplier,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, api.ErrInternal):
		return http.StatusInternalServerError
	case oversized(err):
		return http.StatusRequestEntityTooLarge
	case askedForSomethingWrong(err):
		return http.StatusBadRequest
	case errors.Is(err, api.ErrNotOpen):
		return http.StatusNotFound
	case errors.Is(err, jsonschema.ErrNoSchema):
		return http.StatusNotFound
	case errors.Is(err, helm.ErrNoRelease):
		return http.StatusNotFound
	case errors.Is(err, helm.ErrFluxManaged):
		return http.StatusConflict
	case errors.Is(err, resources.ErrOutOfScope):
		return http.StatusForbidden
	case errors.Is(err, argocd.ErrRefused):
		return http.StatusConflict
	case errors.Is(err, flux.ErrNoSource):
		return http.StatusConflict
	case cannotReachCluster(err):
		return http.StatusServiceUnavailable
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, context.Canceled):
		return http.StatusServiceUnavailable
	default:
		return kubeStatusFor(err)
	}
}

func kubeStatusFor(err error) int {
	switch {
	case apierrors.IsNotFound(err):
		return http.StatusNotFound
	case apierrors.IsConflict(err):
		return http.StatusConflict
	case apierrors.IsUnauthorized(err):
		return http.StatusUnauthorized
	case apierrors.IsForbidden(err):
		return http.StatusForbidden
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return http.StatusUnprocessableEntity
	case unreachable(err):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}
