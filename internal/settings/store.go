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
)

const dirMode = 0o700

const fileMode = 0o600

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
	dir := filepath.Dir(s.path)
	mkdirErr := os.MkdirAll(dir, dirMode)
	if mkdirErr != nil {
		return fmt.Errorf("settings: %w", mkdirErr)
	}
	file, createErr := os.CreateTemp(dir, "settings-*.json")
	if createErr != nil {
		return fmt.Errorf("settings: %w", createErr)
	}
	return s.replace(file, append(body, '\n'))
}

func (s *Store) replace(file *os.File, body []byte) error {
	temp := file.Name()
	_, writeErr := file.Write(body)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("settings: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("settings: %w", closeErr)
	}
	chmodErr := os.Chmod(temp, fileMode)
	if chmodErr != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("settings: %w", chmodErr)
	}
	renameErr := os.Rename(temp, s.path)
	if renameErr != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("settings: %w", renameErr)
	}
	return nil
}
