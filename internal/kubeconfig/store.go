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
)

const dirMode = 0o700

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
	dir := filepath.Dir(s.path)
	mkdirErr := os.MkdirAll(dir, dirMode)
	if mkdirErr != nil {
		return fmt.Errorf("kubeconfig list: %w", mkdirErr)
	}
	file, createErr := os.CreateTemp(dir, "kubeconfigs-*.json")
	if createErr != nil {
		return fmt.Errorf("kubeconfig list: %w", createErr)
	}
	return s.replace(file, append(body, '\n'))
}

func (s *Store) replace(file *os.File, body []byte) error {
	err := fill(file, body)
	if err != nil {
		_ = os.Remove(file.Name())
		return fmt.Errorf("kubeconfig list: %w", err)
	}
	renameErr := os.Rename(file.Name(), s.path)
	if renameErr != nil {
		_ = os.Remove(file.Name())
		return fmt.Errorf("kubeconfig list: %w", renameErr)
	}
	return nil
}

func fill(file *os.File, body []byte) error {
	defer func() { _ = file.Close() }()
	_, err := file.Write(body)
	if err != nil {
		return err
	}
	return file.Sync()
}
