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

func (s *Server) handleHelmRelease(w http.ResponseWriter, r *http.Request) {
	namespace, name, ok := helmCoordinates(w, r)
	if !ok {
		return
	}
	revision, ok := optionalPositive(w, r, "revision", "revision must be a positive number")
	if !ok {
		return
	}
	release, claimed := s.releaseReads.claim(liveIdentity(r), 1)
	if !claimed {
		writeError(w, http.StatusTooManyRequests, "helm release reads are busy; try again")
		return
	}
	defer release()
	detail, err := s.managerFor(r).HelmRelease(r.Context(), namespace, name, revision)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) handleHelmHistory(w http.ResponseWriter, r *http.Request) {
	namespace, name, ok := helmCoordinates(w, r)
	if !ok {
		return
	}
	through, ok := optionalPositive(w, r, "through", "through must be a positive revision")
	if !ok {
		return
	}
	release, claimed := s.releaseReads.claim(liveIdentity(r), 1)
	if !claimed {
		writeError(w, http.StatusTooManyRequests, "helm release reads are busy; try again")
		return
	}
	defer release()
	page, err := s.managerFor(r).HelmHistory(r.Context(), namespace, name, through)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, page)
}

func helmCoordinates(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	query := r.URL.Query()
	namespace := query.Get("namespace")
	name := query.Get("name")
	if namespace == "" || name == "" {
		writeError(w, http.StatusBadRequest, "namespace and name are required")
		return "", "", false
	}
	return namespace, name, true
}

func optionalPositive(
	w http.ResponseWriter,
	r *http.Request,
	key, message string,
) (int64, bool) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed < 1 {
		writeError(w, http.StatusBadRequest, message)
		return 0, false
	}
	return parsed, true
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
	writer, kept, stop, ok := s.writing(w, r, name)
	if !ok {
		return
	}
	defer stop()
	if action == helm.ActionUninstal {
		//nolint:contextcheck // writing detaches r.Context so an abandoned request still finishes the write
		removed, removeErr := writer.HelmUninstall(kept, namespace, name)
		s.finishHelmAction(w, r, releaseChange(verbUninstall, namespace, name, "", false, removeErr), removed, removeErr)
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
	//nolint:contextcheck // writing detaches r.Context so an abandoned request still finishes the write
	rolled, rollErr := writer.HelmRollback(kept, namespace, name, revision)
	detail := "to revision " + strconv.FormatInt(revision, 10)
	s.finishHelmAction(w, r, releaseChange(verbRollback, namespace, name, detail, false, rollErr), rolled, rollErr)
}

func (s *Server) handleHelmVersions(w http.ResponseWriter, r *http.Request) {
	chart := r.URL.Query().Get("chart")
	if chart == "" {
		writeError(w, http.StatusBadRequest, "chart is required")
		return
	}
	release, claimed := s.chartFetches.claim(liveIdentity(r), 1)
	if !claimed {
		writeError(w, http.StatusTooManyRequests, "helm chart searches are busy; try again")
		return
	}
	defer release()
	found, err := s.managerFor(r).HelmVersions(r.Context(), chart)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, found)
}

func (s *Server) handleHelmCharts(w http.ResponseWriter, r *http.Request) {
	release, claimed := s.chartFetches.claim(liveIdentity(r), 1)
	if !claimed {
		writeError(w, http.StatusTooManyRequests, "helm chart searches are busy; try again")
		return
	}
	defer release()
	found, err := s.managerFor(r).HelmChartSearch(r.Context(), r.URL.Query().Get("query"))
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
	release, claimed := s.helmProcesses.claim(liveIdentity(r), 1)
	if !claimed {
		writeError(w, http.StatusTooManyRequests, "helm values reads are busy; try again")
		return
	}
	defer release()
	found, err := s.managerFor(r).HelmChartValues(r.Context(), helm.ValuesRequest{
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
	confirm := dto.Name
	if dryRun {
		confirm = unguarded
	}
	writer, kept, stop, ok := s.writing(w, r, confirm)
	if !ok {
		return
	}
	defer stop()
	//nolint:contextcheck // writing detaches r.Context so an abandoned request still finishes the write
	result, installErr := writer.HelmInstall(kept, helm.InstallRequest{
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
	s.finishHelmAction(w, r, releaseChange(verbInstall, dto.Namespace, dto.Name, dto.Chart+" "+dto.Version, dryRun, installErr), result, installErr)
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
	confirm := dto.Name
	if dryRun {
		confirm = unguarded
	}
	writer, kept, stop, ok := s.writing(w, r, confirm)
	if !ok {
		return
	}
	defer stop()
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
	//nolint:contextcheck // writing detaches r.Context so an abandoned request still finishes the write
	result, upgradeErr := writer.HelmUpgrade(kept, req)
	s.finishHelmAction(w, r, releaseChange(verbUpgrade, dto.Namespace, dto.Name, dto.Chart+" "+dto.Version, dryRun, upgradeErr), result, upgradeErr)
}

func releaseChange(verb, namespace, name, detail string, dryRun bool, err error) change {
	return change{
		verb:   verb,
		ref:    api.ObjectRef{Namespace: namespace, Name: name},
		kind:   kindRelease,
		detail: detail,
		dryRun: dryRun,
		err:    err,
	}
}

func (s *Server) finishHelmAction(
	w http.ResponseWriter,
	r *http.Request,
	made change,
	result api.HelmActionResult,
	err error,
) {
	s.record(r, made)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleHelm(w http.ResponseWriter, r *http.Request) {
	release, claimed := s.releaseReads.claim(liveIdentity(r), 1)
	if !claimed {
		writeError(w, http.StatusTooManyRequests, "helm release reads are busy; try again")
		return
	}
	defer release()
	releases, err := s.managerFor(r).HelmReleases(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, releases)
}
