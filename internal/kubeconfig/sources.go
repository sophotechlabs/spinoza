package kubeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/kube"
)

const noContexts = "this kubeconfig holds no contexts"

type Sources struct {
	fallback string
	resolved string
	store    *Store
}

func NewSources(fallback string, store *Store) *Sources {
	return &Sources{fallback: fallback, resolved: absolute(fallback), store: store}
}

func absolute(path string) string {
	if path == "" {
		return ""
	}
	resolved, err := Resolve(path)
	if err != nil {
		return path
	}
	return resolved
}

func (s *Sources) List() []api.Kubeconfig {
	stored := s.store.Paths()
	out := make([]api.Kubeconfig, 0, len(stored)+1)
	out = append(out, read(api.Kubeconfig{Label: kube.Label(s.fallback)}, s.fallback))
	for _, path := range stored {
		out = append(out, read(api.Kubeconfig{Label: path, Path: path, Removable: true}, path))
	}
	return out
}

func read(entry api.Kubeconfig, file string) api.Kubeconfig {
	entry.Contexts = []api.KubeContext{}
	contexts, err := kube.Read(file)
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	if len(contexts) == 0 {
		entry.Error = noContexts
		return entry
	}
	entry.Contexts = contexts
	return entry
}

func (s *Sources) Add(path string) error {
	resolved, err := s.Resolve(path)
	if err != nil {
		return err
	}
	if resolved == s.resolved {
		return fmt.Errorf("%s is the kubeconfig spinoza already reads by default", resolved)
	}
	contexts, readErr := kube.Read(resolved)
	if readErr != nil {
		return readErr
	}
	if len(contexts) == 0 {
		return errors.New(noContexts)
	}
	return s.store.Add(resolved)
}

func (s *Sources) Remove(path string) error {
	resolved, err := s.Resolve(path)
	if err != nil {
		return err
	}
	return s.store.Remove(resolved)
}

func (s *Sources) Resolve(path string) (string, error) {
	return Resolve(path)
}

func Resolve(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("a kubeconfig path is required")
	}
	expanded, err := expandHome(trimmed)
	if err != nil {
		return "", err
	}
	absolutePath, absErr := filepath.Abs(expanded)
	if absErr != nil {
		return "", fmt.Errorf("kubeconfig %s: %w", trimmed, absErr)
	}
	return absolutePath, nil
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("kubeconfig %s: %w", path, err)
	}
	return filepath.Join(home, path[1:]), nil
}
