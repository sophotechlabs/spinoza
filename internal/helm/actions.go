package helm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	DefaultBinary  = "helm"
	actionTimeout  = 5 * time.Minute
	driverEnv      = "HELM_DRIVER"
	ActionRollback = "rollback"
	ActionUninstal = "uninstall"
)

var nameFormat = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

var ErrNoHelmBinary = errors.New("helm was not found on PATH")

type Runner interface {
	Run(ctx context.Context, args, env []string) (string, error)
	Available() error
}

type helmRunner struct {
	binary string
}

func NewRunner(binary string) Runner {
	if binary == "" {
		binary = DefaultBinary
	}
	return &helmRunner{binary: binary}
}

func (h *helmRunner) Available() error {
	_, err := exec.LookPath(h.binary)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNoHelmBinary, h.binary)
	}
	return nil
}

func (h *helmRunner) Run(ctx context.Context, args, env []string) (string, error) {
	//nolint:gosec // the binary is a flag, every argument is validated below and passed as argv, never through a shell
	command := exec.CommandContext(ctx, h.binary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.Stdin = nil
	command.Env = append(os.Environ(), env...)

	err := command.Run()
	if err == nil {
		return strings.TrimSpace(stdout.String()), nil
	}
	var missing *exec.Error
	if errors.As(err, &missing) {
		return "", fmt.Errorf("%w: %s", ErrNoHelmBinary, h.binary)
	}
	message := firstMeaningful(stderr.String())
	if message == "" {
		return "", err
	}
	return "", errors.New(message)
}

func firstMeaningful(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		kept = append(kept, trimmed)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "; ")
}

func (s *Service) Support() api.HelmSupport {
	support := api.HelmSupport{Binary: DefaultBinary}
	if s.runner == nil {
		support.Reason = "helm actions are not wired up"
		return support
	}
	err := s.runner.Available()
	if err != nil {
		support.Reason = err.Error()
		return support
	}
	support.Available = true
	return support
}

func (s *Service) Rollback(ctx context.Context, namespace, name string, revision int64) (api.HelmActionResult, error) {
	err := s.admits(namespace, name)
	if err != nil {
		return api.HelmActionResult{}, err
	}
	if revision < 1 {
		return api.HelmActionResult{}, errors.New("a rollback needs the revision to go back to")
	}
	driver, driverErr := s.driverFor(ctx, namespace, name)
	if driverErr != nil {
		return api.HelmActionResult{}, driverErr
	}
	args := s.args("rollback", name, strconv.FormatInt(revision, 10), "--namespace", namespace)
	out, runErr := s.run(ctx, args, driver)
	if runErr != nil {
		return api.HelmActionResult{}, runErr
	}
	return api.HelmActionResult{
		Action:   ActionRollback,
		Message:  orDefault(out, fmt.Sprintf("rolled %s back to revision %d", name, revision)),
		Revision: revision,
	}, nil
}

func (s *Service) Uninstall(ctx context.Context, namespace, name string) (api.HelmActionResult, error) {
	err := s.admits(namespace, name)
	if err != nil {
		return api.HelmActionResult{}, err
	}
	driver, driverErr := s.driverFor(ctx, namespace, name)
	if driverErr != nil {
		return api.HelmActionResult{}, driverErr
	}
	args := s.args("uninstall", name, "--namespace", namespace)
	out, runErr := s.run(ctx, args, driver)
	if runErr != nil {
		return api.HelmActionResult{}, runErr
	}
	return api.HelmActionResult{
		Action:  ActionUninstal,
		Message: orDefault(out, fmt.Sprintf("uninstalled %s from %s", name, namespace)),
	}, nil
}

func (s *Service) admits(namespace, name string) error {
	if s.runner == nil {
		return errors.New("helm actions are not wired up")
	}
	if !nameFormat.MatchString(namespace) || !nameFormat.MatchString(name) {
		return errors.New("namespace and release must be valid kubernetes names")
	}
	return nil
}

func (s *Service) args(rest ...string) []string {
	args := append([]string{}, rest...)
	if s.kubeRef.Name != "" {
		args = append(args, "--kube-context", s.kubeRef.Name)
	}
	if s.kubeRef.Kubeconfig != "" {
		args = append(args, "--kubeconfig", s.kubeRef.Kubeconfig)
	}
	return args
}

func (s *Service) run(ctx context.Context, args []string, driver string) (string, error) {
	bounded, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	env := []string{}
	if driver == DriverConfigMap {
		env = append(env, driverEnv+"="+DriverConfigMap)
	}
	return s.runner.Run(bounded, args, env)
}

func (s *Service) driverFor(ctx context.Context, namespace, name string) (string, error) {
	bounded, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	revisions, err := revisionsIn(bounded, s.cs, namespace, name)
	if err != nil {
		return "", err
	}
	for _, item := range revisions {
		return item.driver, nil
	}
	return "", fmt.Errorf("%w: %s/%s", ErrNoRelease, namespace, name)
}

func orDefault(out, fallback string) string {
	if out == "" {
		return fallback
	}
	return out
}
