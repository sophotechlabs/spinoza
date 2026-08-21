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

const enabled = "on"

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
	return maps.Clone(s.values)
}

func (s *Store) On(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key] == enabled
}

func (s *Store) Replace(values map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := maps.Clone(values)
	if next == nil {
		next = map[string]string{}
	}
	err := s.write(next)
	if err != nil {
		return err
	}
	s.values = next
	return nil
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
