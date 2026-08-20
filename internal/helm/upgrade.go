package helm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
)

const ActionUpgrade = "upgrade"

var ErrFluxManaged = errors.New("this release is managed by flux")

type UpgradeRequest struct {
	Namespace string
	Name      string
	Chart     string
	Version   string
	RepoURL   string
	OCI       bool
	Values    string
	DryRun    bool
}

func (s *Service) Upgrade(ctx context.Context, req UpgradeRequest) (api.HelmActionResult, error) {
	err := s.admitsUpgrade(req)
	if err != nil {
		return api.HelmActionResult{}, err
	}
	driver, driverErr := s.driverFor(ctx, req.Namespace, req.Name)
	if driverErr != nil {
		return api.HelmActionResult{}, driverErr
	}
	valuesFile, valuesErr := writeValues(req.Values)
	if valuesErr != nil {
		return api.HelmActionResult{}, valuesErr
	}
	defer func() {
		_ = os.Remove(valuesFile)
	}()
	args := s.args(upgradeArgs(req, valuesFile)...)
	out, runErr := s.run(ctx, args, driver)
	if runErr != nil {
		return api.HelmActionResult{}, runErr
	}
	if req.DryRun {
		return renderResult(ActionUpgrade, req.Chart, req.Version, true, out)
	}
	return api.HelmActionResult{
		Action:  ActionUpgrade,
		Message: orDefault(out, fmt.Sprintf("upgraded %s to %s %s", req.Name, req.Chart, req.Version)),
	}, nil
}

func (s *Service) admitsUpgrade(req UpgradeRequest) error {
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
	repoErr := charts.CheckRepoURL(req.RepoURL)
	if repoErr != nil {
		return repoErr
	}
	return checkValues(req.Values)
}

func checkValues(values string) error {
	if values == "" {
		return nil
	}
	var doc map[string]any
	err := yaml.Unmarshal([]byte(values), &doc)
	if err != nil {
		return fmt.Errorf("values must be a yaml mapping: %w", err)
	}
	return nil
}

func writeValues(values string) (string, error) {
	file, err := os.CreateTemp("", "spinoza-values-*.yaml")
	if err != nil {
		return "", err
	}
	_, writeErr := file.WriteString(values)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(file.Name())
		return "", writeErr
	}
	if closeErr != nil {
		_ = os.Remove(file.Name())
		return "", closeErr
	}
	return file.Name(), nil
}

func upgradeArgs(req UpgradeRequest, valuesFile string) []string {
	args := []string{
		"upgrade",
		req.Name,
		chartRef(req.Chart, req.RepoURL, req.OCI),
		"--namespace", req.Namespace,
		"--version", req.Version,
	}
	if !req.OCI {
		args = append(args, "--repo", req.RepoURL)
	}
	args = append(args, "--values", valuesFile)
	if req.DryRun {
		args = append(args, "--dry-run=server", "--output", "json")
	}
	return args
}

func chartRef(chart, repoURL string, oci bool) string {
	if oci {
		return strings.TrimSuffix(repoURL, "/") + "/" + chart
	}
	return chart
}

func renderResult(action, chart, version string, onCluster bool, out string) (api.HelmActionResult, error) {
	var rendered payload
	err := json.Unmarshal([]byte(afterThePull(out)), &rendered)
	if err != nil {
		return api.HelmActionResult{}, fmt.Errorf("could not read helm's dry-run output: %w", err)
	}
	return api.HelmActionResult{
		Action:   action,
		DryRun:   true,
		Manifest: rendered.Manifest,
		Message:  fmt.Sprintf("%s render of %s %s", renderedBy(onCluster), chart, version),
	}, nil
}

func afterThePull(out string) string {
	start := strings.Index(out, "{")
	if start < 0 {
		return out
	}
	return out[start:]
}

func renderedBy(onCluster bool) string {
	if onCluster {
		return "server"
	}
	return "local"
}
