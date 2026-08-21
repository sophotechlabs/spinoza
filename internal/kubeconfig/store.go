package kubeconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/atomicfile"
)

type state struct {
	Kubeconfigs []string `json:"kubeconfigs"`
}

type Store struct {
	mu    sync.Mutex
	path  string
	paths []string
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("kubeconfig list: %w", err)
	}
	return filepath.Join(dir, "spinoza", "kubeconfigs.json"), nil
}

func Open(path string) (*Store, error) {
	store := &Store{path: path}
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("kubeconfig list %s: %w", path, err)
	}
	var saved state
	unmarshalErr := json.Unmarshal(body, &saved)
	if unmarshalErr != nil {
		return store, fmt.Errorf("kubeconfig list %s: %w", path, unmarshalErr)
	}
	store.paths = saved.Kubeconfigs
	return store, nil
}

func (s *Store) Paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.paths)
}

func (s *Store) Add(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.paths, path) {
		return fmt.Errorf("%s is already on the list", path)
	}
	next := append(slices.Clone(s.paths), path)
	err := s.write(next)
	if err != nil {
		return err
	}
	s.paths = next
	return nil
}

func (s *Store) Remove(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.DeleteFunc(slices.Clone(s.paths), func(kept string) bool {
		return kept == path
	})
	if len(next) == len(s.paths) {
		return fmt.Errorf("%s is not on the list", path)
	}
	err := s.write(next)
	if err != nil {
		return err
	}
	s.paths = next
	return nil
}

func (s *Store) write(paths []string) error {
	if s.path == "" {
		return nil
	}
	body, err := json.MarshalIndent(state{Kubeconfigs: paths}, "", "  ")
	if err != nil {
		return fmt.Errorf("kubeconfig list: %w", err)
	}
	saveErr := atomicfile.Save(s.path, "kubeconfigs-*.json", append(body, '\n'))
	if saveErr != nil {
		return fmt.Errorf("kubeconfig list: %w", saveErr)
	}
	return nil
}
