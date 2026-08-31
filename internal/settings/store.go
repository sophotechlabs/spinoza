package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/atomicfile"
)

const NodeShellKey = "spinoza.nodeshell.v1"

const UpdateCheckKey = "spinoza.update.check.v1"

const ColumnsKey = "spinoza.columns.v1"

const (
	enabled  = "on"
	disabled = "off"
)

type state struct {
	Values map[string]string `json:"values"`
}

type Store struct {
	mu     sync.Mutex
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

func Open(path string) (*Store, error) {
	store := &Store{path: path, values: map[string]string{}}
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("settings %s: %w", path, err)
	}
	var saved state
	unmarshalErr := json.Unmarshal(body, &saved)
	if unmarshalErr != nil {
		return store, fmt.Errorf("settings %s: %w", path, unmarshalErr)
	}
	maps.Copy(store.values, saved.Values)
	return store, nil
}

func Memory() *Store {
	return &Store{values: map[string]string{}}
}

func (s *Store) All() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = s.onDisk()
	return maps.Clone(s.values)
}

func (s *Store) On(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key] == enabled
}

func (s *Store) Off(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key] == disabled
}

func (s *Store) Merge(values map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.onDisk()
	maps.Copy(next, values)
	err := s.write(next)
	if err != nil {
		return err
	}
	s.values = next
	return nil
}

func (s *Store) onDisk() map[string]string {
	held := maps.Clone(s.values)
	if held == nil {
		held = map[string]string{}
	}
	if s.path == "" {
		return held
	}
	body, err := os.ReadFile(s.path)
	if err != nil {
		return held
	}
	var saved state
	if json.Unmarshal(body, &saved) != nil {
		return held
	}
	found := map[string]string{}
	maps.Copy(found, saved.Values)
	return found
}

func (s *Store) write(values map[string]string) error {
	if s.path == "" {
		return nil
	}
	body, err := json.MarshalIndent(state{Values: values}, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	saveErr := atomicfile.Save(s.path, "settings-*.json", append(body, '\n'))
	if saveErr != nil {
		return fmt.Errorf("settings: %w", saveErr)
	}
	return nil
}
