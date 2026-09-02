package kubeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/atomicfile"
	"github.com/sophotechlabs/spinoza/internal/filetx"
)

const maxFileBytes = 1 << 20

type state struct {
	Kubeconfigs []string `json:"kubeconfigs"`
}

type Store struct {
	mu    sync.Mutex
	ctx   context.Context
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

func Open(ctx context.Context, path string) (*Store, error) {
	store := &Store{ctx: ctx, path: path}
	paths, err := readStore(path)
	if err != nil {
		return store, fmt.Errorf("kubeconfig list %s: %w", path, err)
	}
	store.paths = paths
	return store, nil
}

func (s *Store) Paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return slices.Clone(s.paths)
}

func (s *Store) Add(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return s.add(path)
	}
	err := filetx.Exclusive(s.ctx, s.path, func() error {
		paths, readErr := readStore(s.path)
		if readErr != nil {
			return fmt.Errorf("%s: %w", s.path, readErr)
		}
		s.paths = paths
		return s.add(path)
	})
	if err != nil {
		return fmt.Errorf("kubeconfig list: %w", err)
	}
	return nil
}

func (s *Store) Remove(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return s.remove(path)
	}
	err := filetx.Exclusive(s.ctx, s.path, func() error {
		paths, readErr := readStore(s.path)
		if readErr != nil {
			return fmt.Errorf("%s: %w", s.path, readErr)
		}
		s.paths = paths
		return s.remove(path)
	})
	if err != nil {
		return fmt.Errorf("kubeconfig list: %w", err)
	}
	return nil
}

func (s *Store) add(path string) error {
	if slices.Contains(s.paths, path) {
		return fmt.Errorf("%s is already on the list", path)
	}
	next := append(slices.Clone(s.paths), path)
	if err := s.write(next); err != nil {
		return err
	}
	s.paths = next
	return nil
}

func (s *Store) remove(path string) error {
	next := slices.DeleteFunc(slices.Clone(s.paths), func(kept string) bool {
		return kept == path
	})
	if len(next) == len(s.paths) {
		return fmt.Errorf("%s is not on the list", path)
	}
	if err := s.write(next); err != nil {
		return err
	}
	s.paths = next
	return nil
}

func readStore(path string) ([]string, error) {
	body, err := filetx.Read(path, maxFileBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var saved state
	if err := json.Unmarshal(body, &saved); err != nil {
		return nil, err
	}
	return slices.Clone(saved.Kubeconfigs), nil
}

func (s *Store) refresh() {
	if s.path == "" {
		return
	}
	paths, err := readStore(s.path)
	if err == nil {
		s.paths = paths
	}
}

func (s *Store) write(paths []string) error {
	if s.path == "" {
		return nil
	}
	body, err := json.MarshalIndent(state{Kubeconfigs: paths}, "", "  ")
	if err != nil {
		return fmt.Errorf("kubeconfig list: %w", err)
	}
	if len(body)+1 > maxFileBytes {
		return fmt.Errorf("kubeconfig list is larger than %d bytes", maxFileBytes)
	}
	saveErr := atomicfile.Save(s.path, "kubeconfigs-*.json", append(body, '\n'))
	if saveErr != nil {
		return fmt.Errorf("kubeconfig list: %w", saveErr)
	}
	return nil
}
