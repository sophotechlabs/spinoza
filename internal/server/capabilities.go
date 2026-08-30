package server

import (
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const noClusterForCapability = "spinoza has no cluster; pick a context that answers"

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	out := api.Capabilities{
		Helm:       api.HelmSupport{Reason: noClusterForCapability},
		Traffic:    api.TrafficSupport{Reason: noClusterForCapability},
		LocalShell: api.LocalShell{Reason: noLocalShell},
	}
	if s.localShellOpener() != nil {
		out.LocalShell = api.LocalShell{Available: true}
	}
	backend := s.managerFor(r)
	if backend != nil {
		out.Helm = backend.HelmSupport()
		out.Traffic = backend.TrafficSupport(r.Context())
	}
	writeJSON(w, out)
}
