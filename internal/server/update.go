package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/settings"
	"github.com/sophotechlabs/spinoza/internal/update"
)

type Updates interface {
	Status(ctx context.Context) api.UpdateStatus
	Recheck(ctx context.Context) api.UpdateStatus
}

type Installs interface {
	Install(ctx context.Context) error
}

func (s *Server) UseUpdates(checker Updates) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = checker
}

func (s *Server) UseInstaller(installer Installs) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.installer = installer
}

func (s *Server) updateChecker() Updates {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updates
}

func (s *Server) updateInstaller() Installs {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.installer
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	checker := s.updateChecker()
	if checker == nil {
		writeJSON(w, api.UpdateStatus{Reason: "this build does not check for releases"})
		return
	}
	if s.stored().Off(settings.UpdateCheckKey) {
		writeJSON(w, api.UpdateStatus{Reason: "automatic checks are turned off in settings"})
		return
	}
	writeJSON(w, checker.Status(r.Context()))
}

func (s *Server) handleInstallUpdate(w http.ResponseWriter, r *http.Request) {
	checker := s.updateChecker()
	if checker == nil {
		writeJSON(w, api.UpdateResult{Reason: "this build does not check for releases"})
		return
	}
	found := checker.Recheck(r.Context())
	result := api.UpdateResult{Current: found.Current, Latest: found.Latest}
	if !found.Available {
		result.Reason = found.Reason
		writeJSON(w, result)
		return
	}
	installer := s.updateInstaller()
	if installer == nil {
		result.Command = update.InstallCommand()
		result.Reason = update.ErrUnsupported.Error()
		writeJSON(w, result)
		return
	}
	kept, stop := context.WithTimeout(context.WithoutCancel(r.Context()), mutationTimeout)
	defer stop()
	err := installer.Install(kept)
	if err == nil {
		result.Updated = true
		writeJSON(w, result)
		return
	}
	result.Reason = err.Error()
	if errors.Is(err, update.ErrUnsupported) {
		result.Command = update.InstallCommand()
	}
	writeJSON(w, result)
}
