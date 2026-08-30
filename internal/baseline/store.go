package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/atomicfile"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

// A baseline of a large cluster is tens of thousands of fingerprints. This is
// the point past which the file is refused rather than written: something has
// gone wrong upstream, and an unbounded file in a config directory is worse
// than a missing baseline.
const maxKeys = 500_000

const maxBytes = 64 << 20

const nameLength = 16

type stored struct {
	// Cluster is where the baseline was taken, kept so one handed to another
	// cluster says whose it was rather than pretending to be its own.
	Cluster string            `json:"cluster,omitempty"`
	TakenAt string            `json:"takenAt"`
	Checks  []string          `json:"checks"`
	Counts  map[string]int    `json:"counts"`
	Scanned int               `json:"scanned,omitempty"`
	Keys    map[string]string `json:"keys"`
}

// Store keeps one baseline per cluster, as a file of its own. It is deliberately
// not in the settings file: that one is read on every checks request and
// rewritten whole whenever any setting moves.
type Store struct {
	mu  sync.Mutex
	dir string
}

func Open(dir string) *Store {
	return &Store{dir: dir}
}

func DefaultDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("baselines: %w", err)
	}
	return filepath.Join(dir, "spinoza", "baselines"), nil
}

// fileFor names the file after a hash of the cluster, so an apiserver URL never
// has to survive being a path.
func (s *Store) fileFor(cluster string) string {
	sum := sha256.Sum256([]byte(cluster))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])[:nameLength]+".json")
}

func (s *Store) Load(cluster string) (checks.Baseline, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return checks.Baseline{}, false
	}
	body, err := os.ReadFile(s.fileFor(cluster))
	if err != nil {
		return checks.Baseline{}, false
	}
	if len(body) > maxBytes {
		return checks.Baseline{}, false
	}
	var held stored
	if json.Unmarshal(body, &held) != nil {
		return checks.Baseline{}, false
	}
	keys := held.Keys
	if keys == nil {
		keys = map[string]string{}
	}
	return checks.Baseline{
		TakenAt: held.TakenAt,
		Cluster: held.Cluster,
		Checks:  held.Checks,
		Counts:  held.Counts,
		Scanned: held.Scanned,
		Keys:    keys,
	}, true
}

func (s *Store) Save(cluster string, taken checks.Baseline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return nil
	}
	if len(taken.Keys) > maxKeys {
		return fmt.Errorf("baselines: %d findings is more than one baseline holds", len(taken.Keys))
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("baselines: %w", err)
	}
	body, err := json.Marshal(flatten(taken))
	if err != nil {
		return fmt.Errorf("baselines: %w", err)
	}
	if saveErr := atomicfile.Save(s.fileFor(cluster), "baseline-*.json", body); saveErr != nil {
		return fmt.Errorf("baselines: %w", saveErr)
	}
	return nil
}

func (s *Store) Clear(cluster string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return nil
	}
	err := os.Remove(s.fileFor(cluster))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("baselines: %w", err)
	}
	return nil
}

func flatten(taken checks.Baseline) stored {
	return stored{
		Cluster: taken.Cluster,
		TakenAt: taken.TakenAt,
		Checks:  taken.Checks,
		Counts:  taken.Counts,
		Scanned: taken.Scanned,
		Keys:    taken.Keys,
	}
}

// Encode writes a baseline the way it is kept on disk, so one can be handed to
// somebody else or kept in a repository.
func Encode(taken checks.Baseline) ([]byte, error) {
	body, err := json.Marshal(flatten(taken))
	if err != nil {
		return nil, fmt.Errorf("baselines: %w", err)
	}
	return body, nil
}

// Decode reads one back. Anything that is not a baseline is refused rather
// than stored: a file that turns out to be empty would quietly report every
// finding in the cluster as new.
func Decode(body []byte) (checks.Baseline, error) {
	if len(body) > maxBytes {
		return checks.Baseline{}, fmt.Errorf("baselines: %d bytes is more than one baseline holds", len(body))
	}
	var held stored
	if err := json.Unmarshal(body, &held); err != nil {
		return checks.Baseline{}, fmt.Errorf("baselines: this is not a baseline: %w", err)
	}
	if held.TakenAt == "" || len(held.Checks) == 0 {
		return checks.Baseline{}, errors.New("baselines: this file names no checks and no day it was taken")
	}
	keys := held.Keys
	if keys == nil {
		keys = map[string]string{}
	}
	return checks.Baseline{
		TakenAt: held.TakenAt,
		Cluster: held.Cluster,
		Checks:  held.Checks,
		Counts:  held.Counts,
		Scanned: held.Scanned,
		Keys:    keys,
	}, nil
}
