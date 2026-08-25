package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

func (s *Server) handleHelmSupport(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.manager().HelmSupport())
}

func (s *Server) handleHelmRelease(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	namespace := query.Get("namespace")
	name := query.Get("name")
	if namespace == "" || name == "" {
		writeError(w, http.StatusBadRequest, "namespace and name are required")
		return
	}
	detail, err := s.manager().HelmRelease(r.Context(), namespace, name)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) handleHelmAction(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	namespace := query.Get("namespace")
	name := query.Get("name")
	if namespace == "" || name == "" {
		writeError(w, http.StatusBadRequest, "namespace and name are required")
		return
	}
	action := query.Get("action")
	if s.unconfirmed(r, name) {
		refuseUnconfirmed(w, name)
		return
	}
	if action == helm.ActionUninstal {
		removed, removeErr := s.manager().HelmUninstall(r.Context(), namespace, name)
		s.finishHelmAction(w, removed, removeErr)
		return
	}
	if action != helm.ActionRollback {
		writeError(w, http.StatusBadRequest, "action must be rollback or uninstall")
		return
	}
	revision, err := strconv.ParseInt(query.Get("revision"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "revision must be a number")
		return
	}
	rolled, rollErr := s.manager().HelmRollback(r.Context(), namespace, name, revision)
	s.finishHelmAction(w, rolled, rollErr)
}

func (s *Server) handleHelmVersions(w http.ResponseWriter, r *http.Request) {
	chart := r.URL.Query().Get("chart")
	if chart == "" {
		writeError(w, http.StatusBadRequest, "chart is required")
		return
	}
	found, err := s.manager().HelmVersions(r.Context(), chart)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, found)
}

func (s *Server) handleHelmCharts(w http.ResponseWriter, r *http.Request) {
	found, err := s.manager().HelmChartSearch(r.Context(), r.URL.Query().Get("query"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, found)
}

func (s *Server) handleHelmChartValues(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	chart := query.Get("chart")
	repo := query.Get("repo")
	wanted := query.Get("version")
	if chart == "" || repo == "" || wanted == "" {
		writeError(w, http.StatusBadRequest, "chart, repo and version are required")
		return
	}
	found, err := s.manager().HelmChartValues(r.Context(), helm.ValuesRequest{
		Chart:   chart,
		Version: wanted,
		RepoURL: repo,
		OCI:     strings.HasPrefix(repo, ociScheme),
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, found)
}

type helmInstallBody struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	Chart           string `json:"chart"`
	Repo            string `json:"repo"`
	Version         string `json:"version"`
	Values          string `json:"values"`
	CreateNamespace bool   `json:"createNamespace"`
}

func (s *Server) handleHelmInstall(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDocBytes))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var dto helmInstallBody
	unmarshalErr := json.Unmarshal(body, &dto)
	if unmarshalErr != nil {
		writeError(w, http.StatusBadRequest, "the install request must be json: "+unmarshalErr.Error())
		return
	}
	if dto.Namespace == "" || dto.Name == "" || dto.Chart == "" || dto.Repo == "" || dto.Version == "" {
		writeError(w, http.StatusBadRequest, "namespace, name, chart, repo and version are required")
		return
	}
	dryRun := r.URL.Query().Get("dryRun") == queryTrue
	if !dryRun && s.unconfirmed(r, dto.Name) {
		refuseUnconfirmed(w, dto.Name)
		return
	}
	result, installErr := s.manager().HelmInstall(r.Context(), helm.InstallRequest{
		Namespace:       dto.Namespace,
		Name:            dto.Name,
		Chart:           dto.Chart,
		Version:         dto.Version,
		RepoURL:         dto.Repo,
		OCI:             strings.HasPrefix(dto.Repo, ociScheme),
		Values:          dto.Values,
		CreateNamespace: dto.CreateNamespace,
		DryRun:          dryRun,
	})
	s.finishHelmAction(w, result, installErr)
}

type helmUpgradeBody struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Chart     string `json:"chart"`
	Repo      string `json:"repo"`
	Version   string `json:"version"`
	Values    string `json:"values"`
}

func (s *Server) handleHelmUpgrade(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDocBytes))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var dto helmUpgradeBody
	unmarshalErr := json.Unmarshal(body, &dto)
	if unmarshalErr != nil {
		writeError(w, http.StatusBadRequest, "the upgrade request must be json: "+unmarshalErr.Error())
		return
	}
	if dto.Namespace == "" || dto.Name == "" || dto.Chart == "" || dto.Repo == "" || dto.Version == "" {
		writeError(w, http.StatusBadRequest, "namespace, name, chart, repo and version are required")
		return
	}
	dryRun := r.URL.Query().Get("dryRun") == queryTrue
	if !dryRun && s.unconfirmed(r, dto.Name) {
		refuseUnconfirmed(w, dto.Name)
		return
	}
	req := helm.UpgradeRequest{
		Namespace: dto.Namespace,
		Name:      dto.Name,
		Chart:     dto.Chart,
		Version:   dto.Version,
		RepoURL:   dto.Repo,
		OCI:       strings.HasPrefix(dto.Repo, ociScheme),
		Values:    dto.Values,
		DryRun:    dryRun,
	}
	result, upgradeErr := s.manager().HelmUpgrade(r.Context(), req)
	s.finishHelmAction(w, result, upgradeErr)
}

func (s *Server) finishHelmAction(w http.ResponseWriter, result api.HelmActionResult, err error) {
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleHelm(w http.ResponseWriter, r *http.Request) {
	releases, err := s.manager().HelmReleases(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, releases)
}
