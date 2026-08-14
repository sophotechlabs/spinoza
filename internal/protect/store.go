package protect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const dirMode = 0o700

type state struct {
	Clusters map[string]bool `json:"clusters"`
}

type Store struct {
	mu       sync.Mutex
	path     string
	clusters map[string]bool
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("protected clusters: %w", err)
	}
	return filepath.Join(dir, "spinoza", "protected.json"), nil
}

func Open(path string) (*Store, error) {
	store := &Store{path: path, clusters: map[string]bool{}}
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("protected clusters %s: %w", path, err)
	}
	var saved state
	unmarshalErr := json.Unmarshal(body, &saved)
	if unmarshalErr != nil {
		return store, fmt.Errorf("protected clusters %s: %w", path, unmarshalErr)
	}
	maps.Copy(store.clusters, saved.Clusters)
	return store, nil
}

func (s *Store) Verdict(server string) string {
	if server == "" {
		return api.ProtectionUnknown
	}
	s.mu.Lock()
	protected, decided := s.clusters[server]
	s.mu.Unlock()
	if decided && protected {
		return api.ProtectionProtected
	}
	if decided {
		return api.ProtectionOpen
	}
	if Local(server) {
		return api.ProtectionOpen
	}
	return api.ProtectionUnknown
}

func (s *Store) Set(server string, protected bool) error {
	if server == "" {
		return errors.New("spinoza is not connected to a cluster")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := maps.Clone(s.clusters)
	next[server] = protected
	err := s.write(next)
	if err != nil {
		return err
	}
	s.clusters = next
	return nil
}

func Local(server string) bool {
	parsed, err := url.Parse(server)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func (s *Store) write(clusters map[string]bool) error {
	if s.path == "" {
		return nil
	}
	body, err := json.MarshalIndent(state{Clusters: clusters}, "", "  ")
	if err != nil {
		return fmt.Errorf("protected clusters: %w", err)
	}
	dir := filepath.Dir(s.path)
	mkdirErr := os.MkdirAll(dir, dirMode)
	if mkdirErr != nil {
		return fmt.Errorf("protected clusters: %w", mkdirErr)
	}
	file, createErr := os.CreateTemp(dir, "protected-*.json")
	if createErr != nil {
		return fmt.Errorf("protected clusters: %w", createErr)
	}
	return s.replace(file, append(body, '\n'))
}

func (s *Store) replace(file *os.File, body []byte) error {
	err := fill(file, body)
	if err != nil {
		_ = os.Remove(file.Name())
		return fmt.Errorf("protected clusters: %w", err)
	}
	renameErr := os.Rename(file.Name(), s.path)
	if renameErr != nil {
		_ = os.Remove(file.Name())
		return fmt.Errorf("protected clusters: %w", renameErr)
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
