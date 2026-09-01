package helm

import (
	"context"
	"fmt"
	"os"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
)

const ActionInstall = "install"

type InstallRequest struct {
	Namespace       string
	Name            string
	Chart           string
	Version         string
	RepoURL         string
	OCI             bool
	Values          string
	CreateNamespace bool
	DryRun          bool
}

func (s *Service) Install(ctx context.Context, req InstallRequest) (api.HelmActionResult, error) {
	err := s.admitsInstall(req)
	if err != nil {
		return api.HelmActionResult{}, err
	}
	valuesFile, valuesErr := writeValues(req.Values)
	if valuesErr != nil {
		return api.HelmActionResult{}, valuesErr
	}
	defer func() {
		_ = os.Remove(valuesFile)
	}()
	onCluster := s.namespaceExists(ctx, req.Namespace)
	args := s.args(ctx, installArgs(req, valuesFile, onCluster)...)
	out, runErr := s.run(ctx, args, "")
	if runErr != nil {
		return api.HelmActionResult{}, runErr
	}
	if req.DryRun {
		return renderResult(ActionInstall, req.Chart, req.Version, onCluster, out)
	}
	return api.HelmActionResult{
		Action:  ActionInstall,
		Message: orDefault(out, fmt.Sprintf("installed %s %s as %s in %s", req.Chart, req.Version, req.Name, req.Namespace)),
	}, nil
}

func (s *Service) admitsInstall(req InstallRequest) error {
	err := s.admits(req.Namespace, req.Name)
	if err != nil {
		return err
	}
	if !nameFormat.MatchString(req.Chart) {
		return fmt.Errorf("%q is not a chart name", req.Chart)
	}
	if !charts.ValidVersion(req.Version) {
		return fmt.Errorf("version %q is not a semantic version", req.Version)
	}
	repoErr := s.admitsRepository(req.RepoURL, req.OCI)
	if repoErr != nil {
		return repoErr
	}
	return checkValues(req.Values)
}

func (s *Service) namespaceExists(ctx context.Context, namespace string) bool {
	if s.cs == nil {
		return false
	}
	bounded, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	_, err := s.cs.CoreV1().Namespaces().Get(bounded, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false
	}
	return err == nil
}

func installArgs(req InstallRequest, valuesFile string, onCluster bool) []string {
	args := []string{
		"install",
		req.Name,
		chartRef(req.Chart, req.RepoURL, req.OCI),
		"--namespace", req.Namespace,
		"--version", req.Version,
	}
	if !req.OCI {
		args = append(args, "--repo", req.RepoURL)
	}
	args = append(args, "--values", valuesFile)
	if req.CreateNamespace {
		args = append(args, "--create-namespace")
	}
	if req.DryRun {
		args = append(args, dryRunFlag(onCluster), "--output", "json")
	}
	return args
}

func dryRunFlag(onCluster bool) string {
	if onCluster {
		return "--dry-run=server"
	}
	return "--dry-run=client"
}
