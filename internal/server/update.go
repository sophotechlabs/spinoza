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

// Installs replaces the running binary. A build that cannot has none wired up.
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

// handleUpdate reports whether there is a newer release. A build with no checker
// says so in the reason, and so does a run the user has turned checking off in.
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

// handleInstallUpdate asks again and installs what it finds. Pressing a button
// is a reason to ask whatever the setting says.
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
	err := installer.Install(r.Context())
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
