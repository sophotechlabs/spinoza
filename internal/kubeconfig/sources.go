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
	defaults map[string]struct{}
	store    *Store
}

func NewSources(fallback string, store *Store) *Sources {
	defaults := make(map[string]struct{})
	for _, path := range kube.Paths(fallback) {
		defaults[absolute(path)] = struct{}{}
	}
	return &Sources{fallback: fallback, defaults: defaults, store: store}
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
		if s.isDefault(path) {
			continue
		}
		out = append(out, read(api.Kubeconfig{Label: path, Path: path, Removable: true}, path))
	}
	return out
}

func (s *Sources) isDefault(path string) bool {
	_, found := s.defaults[absolute(path)]
	return found
}

func (s *Sources) PublicPath(path string) string {
	if s.isDefault(path) {
		return ""
	}
	return path
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
	if s.isDefault(resolved) {
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
	return resolveWith(path, filepath.Abs)
}

func resolveWith(path string, makeAbsolute func(string) (string, error)) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("a kubeconfig path is required")
	}
	expanded, err := expandHome(trimmed)
	if err != nil {
		return "", err
	}
	absolutePath, absErr := makeAbsolute(expanded)
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
