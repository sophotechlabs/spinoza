package protect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/atomicfile"
	"github.com/sophotechlabs/spinoza/internal/clusterid"
	"github.com/sophotechlabs/spinoza/internal/filetx"
)

const maxFileBytes = 1 << 20

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
	clusters, err := read(path)
	if err != nil {
		return store, fmt.Errorf("protected clusters %s: %w", path, err)
	}
	store.clusters = clusters
	return store, nil
}

func adopt(into, saved map[string]bool) {
	for server, protected := range saved {
		key := clusterid.Normalize(server)
		if key == "" {
			continue
		}
		if into[key] {
			continue
		}
		into[key] = protected
	}
}

func (s *Store) Verdict(server string) string {
	key := clusterid.Normalize(server)
	if key == "" {
		return api.ProtectionUnknown
	}
	protected, decided := s.decision(key)
	if decided && protected {
		return api.ProtectionProtected
	}
	if decided {
		return api.ProtectionOpen
	}
	if Local(key) {
		return api.ProtectionOpen
	}
	return api.ProtectionUnknown
}

func (s *Store) decision(key string) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	protected, decided := s.clusters[key]
	return protected, decided
}

func (s *Store) Set(server string, protected bool) error {
	key := clusterid.Normalize(server)
	if key == "" {
		return errors.New("spinoza is not connected to a cluster")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		if s.clusters == nil {
			s.clusters = map[string]bool{}
		}
		s.clusters[key] = protected
		return nil
	}
	err := filetx.Exclusive(s.path, func() error {
		next, readErr := read(s.path)
		if readErr != nil {
			return fmt.Errorf("%s: %w", s.path, readErr)
		}
		next[key] = protected
		if err := s.write(next); err != nil {
			return err
		}
		s.clusters = next
		return nil
	})
	if err != nil {
		return fmt.Errorf("protected clusters: %w", err)
	}
	return nil
}

func read(path string) (map[string]bool, error) {
	body, err := filetx.Read(path, maxFileBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var saved state
	if err := json.Unmarshal(body, &saved); err != nil {
		return nil, err
	}
	found := map[string]bool{}
	adopt(found, saved.Clusters)
	return found, nil
}

func (s *Store) refresh() {
	if s.path == "" {
		return
	}
	found, err := read(s.path)
	if err == nil {
		s.clusters = found
	}
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
	if len(body)+1 > maxFileBytes {
		return fmt.Errorf("protected clusters are larger than %d bytes", maxFileBytes)
	}
	saveErr := atomicfile.Save(s.path, "protected-*.json", append(body, '\n'))
	if saveErr != nil {
		return fmt.Errorf("protected clusters: %w", saveErr)
	}
	return nil
}
