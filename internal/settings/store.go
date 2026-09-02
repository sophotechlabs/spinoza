package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/atomicfile"
	"github.com/sophotechlabs/spinoza/internal/filetx"
)

const NodeShellKey = "spinoza.nodeshell.v1"

const UpdateCheckKey = "spinoza.update.check.v1"

const ColumnsKey = "spinoza.columns.v1"

const (
	enabled      = "on"
	disabled     = "off"
	maxFileBytes = 1 << 20
)

type state struct {
	Values map[string]string `json:"values"`
}

type Store struct {
	mu     sync.Mutex
	ctx    context.Context
	path   string
	values map[string]string
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("settings: %w", err)
	}
	return filepath.Join(dir, "spinoza", "settings.json"), nil
}

func Open(ctx context.Context, path string) (*Store, error) {
	store := &Store{ctx: ctx, path: path, values: map[string]string{}}
	values, err := read(path)
	if err != nil {
		return store, fmt.Errorf("settings %s: %w", path, err)
	}
	store.values = values
	return store, nil
}

func Memory() *Store {
	return &Store{values: map[string]string{}}
}

func (s *Store) All() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return maps.Clone(s.values)
}

func (s *Store) On(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return s.values[key] == enabled
}

func (s *Store) Off(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return s.values[key] == disabled
}

func (s *Store) Merge(values map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		if s.values == nil {
			s.values = map[string]string{}
		}
		maps.Copy(s.values, values)
		return nil
	}
	err := filetx.Exclusive(s.ctx, s.path, func() error {
		next, readErr := read(s.path)
		if readErr != nil {
			return fmt.Errorf("%s: %w", s.path, readErr)
		}
		maps.Copy(next, values)
		if err := s.write(next); err != nil {
			return err
		}
		s.values = next
		return nil
	})
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return nil
}

func read(path string) (map[string]string, error) {
	body, err := filetx.Read(path, maxFileBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var saved state
	if err := json.Unmarshal(body, &saved); err != nil {
		return nil, err
	}
	found := map[string]string{}
	maps.Copy(found, saved.Values)
	return found, nil
}

func (s *Store) refresh() {
	if s.path == "" {
		return
	}
	found, err := read(s.path)
	if err == nil {
		s.values = found
	}
}

func (s *Store) write(values map[string]string) error {
	if s.path == "" {
		return nil
	}
	body, err := json.MarshalIndent(state{Values: values}, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if len(body)+1 > maxFileBytes {
		return fmt.Errorf("settings are larger than %d bytes", maxFileBytes)
	}
	saveErr := atomicfile.Save(s.path, "settings-*.json", append(body, '\n'))
	if saveErr != nil {
		return fmt.Errorf("settings: %w", saveErr)
	}
	return nil
}
