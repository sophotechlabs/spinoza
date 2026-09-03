package baseline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/atomicfile"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/filetx"
)

const maxKeys = 50_000

const maxBytes = 8 << 20

const maxKeyValueBytes = 4 << 20

const maxChecks = 512

const maxCounts = 512

const nameLength = 16

var ErrRead = errors.New("baselines: could not be read")

type errorReader struct {
	reader io.Reader
	err    error
}

func (r *errorReader) Read(body []byte) (int, error) {
	read, err := r.reader.Read(body)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}
	return read, err
}

type stored struct {
	Cluster string            `json:"cluster,omitempty"`
	TakenAt string            `json:"takenAt"`
	Checks  []string          `json:"checks"`
	Counts  map[string]int    `json:"counts"`
	Scanned int               `json:"scanned,omitempty"`
	Keys    map[string]string `json:"keys"`
}

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
	body, err := filetx.Read(s.fileFor(cluster), maxBytes)
	if err != nil {
		return checks.Baseline{}, false
	}
	taken, err := Decode(body)
	if err != nil {
		return checks.Baseline{}, false
	}
	return taken, true
}

func (s *Store) Save(cluster string, taken checks.Baseline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("baselines: %w", err)
	}
	body, err := Encode(taken)
	if err != nil {
		return err
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

func Encode(taken checks.Baseline) ([]byte, error) {
	if err := validate(taken); err != nil {
		return nil, err
	}
	body, err := json.Marshal(flatten(taken))
	if err != nil {
		return nil, fmt.Errorf("baselines: %w", err)
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("baselines: %d bytes is more than one baseline holds", len(body))
	}
	return body, nil
}

func Decode(body []byte) (checks.Baseline, error) {
	if len(body) > maxBytes {
		return checks.Baseline{}, fmt.Errorf("baselines: %d bytes is more than one baseline holds", len(body))
	}
	return DecodeReader(bytes.NewReader(body))
}

func DecodeReader(reader io.Reader) (checks.Baseline, error) {
	tracked := &errorReader{reader: reader}
	limited := &io.LimitedReader{R: tracked, N: maxBytes + 1}
	held, err := decodeStored(json.NewDecoder(limited))
	if tracked.err != nil {
		return checks.Baseline{}, fmt.Errorf("%w: %w", ErrRead, tracked.err)
	}
	if limited.N == 0 {
		return checks.Baseline{}, fmt.Errorf("baselines: more than %d bytes is more than one baseline holds", maxBytes)
	}
	if err != nil {
		return checks.Baseline{}, fmt.Errorf("baselines: this is not a baseline: %w", err)
	}
	return asBaseline(held)
}

func decodeStored(decoder *json.Decoder) (stored, error) {
	opening, err := decoder.Token()
	if err != nil {
		return stored{}, err
	}
	delim, isDelim := opening.(json.Delim)
	if !isDelim || delim != '{' {
		return stored{}, errors.New("the top-level value is not an object")
	}
	var held stored
	for decoder.More() {
		field, fieldErr := decoder.Token()
		if fieldErr != nil {
			return stored{}, fieldErr
		}
		name, isString := field.(string)
		if !isString {
			return stored{}, errors.New("a baseline field name is not a string")
		}
		if decodeErr := decodeStoredField(decoder, &held, name); decodeErr != nil {
			return stored{}, decodeErr
		}
	}
	if _, closingErr := decoder.Token(); closingErr != nil {
		return stored{}, closingErr
	}
	var trailing any
	trailingErr := decoder.Decode(&trailing)
	if !errors.Is(trailingErr, io.EOF) {
		if trailingErr == nil {
			return stored{}, errors.New("the baseline has a trailing value")
		}
		return stored{}, trailingErr
	}
	return held, nil
}

func decodeStoredField(decoder *json.Decoder, held *stored, name string) error {
	var err error
	switch name {
	case "cluster":
		err = decoder.Decode(&held.Cluster)
	case "takenAt":
		err = decoder.Decode(&held.TakenAt)
	case "checks":
		held.Checks, err = decodeChecks(decoder)
	case "counts":
		held.Counts, err = decodeCounts(decoder)
	case "scanned":
		err = decoder.Decode(&held.Scanned)
	case "keys":
		held.Keys, err = decodeKeys(decoder)
	default:
		return fmt.Errorf("unknown field %q", name)
	}
	return err
}

func decodeChecks(decoder *json.Decoder) ([]string, error) {
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := opening.(json.Delim)
	if !isDelim || delim != '[' {
		return nil, errors.New("baseline checks are not a list")
	}
	names := make([]string, 0, min(maxChecks, 16))
	for decoder.More() {
		if len(names) == maxChecks {
			return nil, fmt.Errorf("%d checks is more than one baseline holds", maxChecks+1)
		}
		var check string
		if err := decoder.Decode(&check); err != nil {
			return nil, err
		}
		names = append(names, check)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return names, nil
}

func decodeCounts(decoder *json.Decoder) (map[string]int, error) {
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if opening == nil {
		return map[string]int{}, nil
	}
	delim, isDelim := opening.(json.Delim)
	if !isDelim || delim != '{' {
		return nil, errors.New("baseline counts are not an object")
	}
	counts := make(map[string]int)
	for decoder.More() {
		if len(counts) == maxCounts {
			return nil, fmt.Errorf("%d counts is more than one baseline holds", maxCounts+1)
		}
		field, fieldErr := decoder.Token()
		if fieldErr != nil {
			return nil, fieldErr
		}
		check, isString := field.(string)
		if !isString {
			return nil, errors.New("a baseline count name is not a string")
		}
		var count int
		if err := decoder.Decode(&count); err != nil {
			return nil, err
		}
		counts[check] = count
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return counts, nil
}

func decodeKeys(decoder *json.Decoder) (map[string]string, error) {
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if opening == nil {
		return map[string]string{}, nil
	}
	delim, isDelim := opening.(json.Delim)
	if !isDelim || delim != '{' {
		return nil, errors.New("baseline keys are not an object")
	}
	keys := make(map[string]string)
	contentBytes := 0
	for decoder.More() {
		if len(keys) == maxKeys {
			return nil, fmt.Errorf("%d findings is more than one baseline holds", maxKeys+1)
		}
		field, fieldErr := decoder.Token()
		if fieldErr != nil {
			return nil, fieldErr
		}
		key, isString := field.(string)
		if !isString {
			return nil, errors.New("a baseline key is not a string")
		}
		var value string
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		contentBytes += len(key) + len(value)
		if contentBytes > maxKeyValueBytes {
			return nil, fmt.Errorf("baseline finding keys and values exceed %d bytes", maxKeyValueBytes)
		}
		keys[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return keys, nil
}

func asBaseline(held stored) (checks.Baseline, error) {
	keys := held.Keys
	if keys == nil {
		keys = map[string]string{}
	}
	taken := checks.Baseline{
		TakenAt: held.TakenAt,
		Cluster: held.Cluster,
		Checks:  held.Checks,
		Counts:  held.Counts,
		Scanned: held.Scanned,
		Keys:    keys,
	}
	if err := validate(taken); err != nil {
		return checks.Baseline{}, err
	}
	return taken, nil
}

func validate(taken checks.Baseline) error {
	if taken.TakenAt == "" || len(taken.Checks) == 0 {
		return errors.New("baselines: this file names no checks and no day it was taken")
	}
	if len(taken.Keys) > maxKeys {
		return fmt.Errorf("baselines: %d findings is more than one baseline holds", len(taken.Keys))
	}
	if len(taken.Checks) > maxChecks {
		return fmt.Errorf("baselines: %d checks is more than one baseline holds", len(taken.Checks))
	}
	if len(taken.Counts) > maxCounts {
		return fmt.Errorf("baselines: %d counts is more than one baseline holds", len(taken.Counts))
	}
	contentBytes := 0
	for key, value := range taken.Keys {
		contentBytes += len(key) + len(value)
		if contentBytes > maxKeyValueBytes {
			return fmt.Errorf("baselines: finding keys and values exceed %d bytes", maxKeyValueBytes)
		}
	}
	return nil
}
