package transcript

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	maxBytes    = 1 << 20
	headerWidth = 512
	fieldWidth  = 120
	ellipsis    = "…"
	extension   = ".session"
	truncated   = "\n[the rest of this session was not recorded: it passed the size this deployment keeps]\n"
)

var errNoDirectory = errors.New("sessions are not being recorded")

var errUnknownSession = errors.New("no session by that name was recorded")

type Header struct {
	ID     string `json:"id"`
	At     string `json:"at"`
	Actor  string `json:"actor"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Bytes  int    `json:"bytes"`
}

type Store struct {
	dir string
}

func DefaultDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("transcript: %w", err)
	}
	return filepath.Join(dir, "spinoza", "sessions"), nil
}

func Open(dir string) *Store {
	if dir == "" {
		return &Store{}
	}
	return &Store{dir: dir}
}

func (s *Store) On() bool {
	return s != nil && s.dir != ""
}

func (s *Store) pathFor(id string) (string, error) {
	if !s.On() {
		return "", errNoDirectory
	}
	if id == "" || strings.ContainsAny(id, `/\.`) {
		return "", errUnknownSession
	}
	return filepath.Join(s.dir, id+extension), nil
}

type Session struct {
	mu      sync.Mutex
	file    *os.File
	header  Header
	path    string
	written int
	full    bool
}

func (s *Store) Start(header Header) (*Session, error) {
	path, err := s.pathFor(header.ID)
	if err != nil {
		return nil, err
	}
	line, encodeErr := headerLine(header)
	if encodeErr != nil {
		return nil, encodeErr
	}
	if mkErr := os.MkdirAll(s.dir, 0o700); mkErr != nil {
		return nil, fmt.Errorf("transcript: %w", mkErr)
	}
	file, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if openErr != nil {
		return nil, fmt.Errorf("transcript: %w", openErr)
	}
	_, writeErr := file.Write(line)
	if writeErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("transcript: %w", writeErr)
	}
	return &Session{file: file, header: header, path: path}, nil
}

func headerLine(header Header) ([]byte, error) {
	header.Actor = shortened(header.Actor)
	header.Target = shortened(header.Target)
	header.Kind = shortened(header.Kind)
	line, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("transcript: %w", err)
	}
	if len(line) >= headerWidth {
		return nil, fmt.Errorf("transcript: this session cannot be described in %d bytes", headerWidth)
	}
	padded := make([]byte, headerWidth)
	copy(padded, line)
	for at := len(line); at < headerWidth-1; at++ {
		padded[at] = ' '
	}
	padded[headerWidth-1] = '\n'
	return padded, nil
}

func shortened(value string) string {
	if len(value) <= fieldWidth {
		return value
	}
	kept := []rune(value)
	for len(string(kept))+len(ellipsis) > fieldWidth {
		kept = kept[:len(kept)-1]
	}
	return string(kept) + ellipsis
}

func (s *Session) Typed(payload []byte) {
	s.write(payload)
}

func (s *Session) Shown(payload []byte) {
	s.write(payload)
}

func (s *Session) write(payload []byte) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil || s.full {
		return
	}
	room := maxBytes - s.written
	if len(payload) > room {
		payload = payload[:room]
	}
	written, err := s.file.Write(payload)
	s.written += written
	if err != nil {
		s.full = true
		return
	}
	if s.written >= maxBytes {
		s.stopWriting()
	}
}

func (s *Session) stopWriting() {
	s.full = true
	_, _ = s.file.WriteString(truncated)
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return
	}
	s.header.Bytes = s.written
	line, err := headerLine(s.header)
	if err == nil {
		_, _ = s.file.WriteAt(line, 0)
	}
	_ = s.file.Close()
	s.file = nil
}

func (s *Store) List(limit int) ([]Header, error) {
	if !s.On() {
		return nil, errNoDirectory
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Header{}, nil
		}
		return nil, fmt.Errorf("transcript: %w", err)
	}
	out := []Header{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != extension {
			continue
		}
		header, headerErr := s.headerOf(entry.Name())
		if headerErr != nil {
			continue
		}
		out = append(out, header)
	}
	slices.SortFunc(out, func(a, b Header) int { return strings.Compare(b.At, a.At) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) headerOf(name string) (Header, error) {
	file, err := os.Open(filepath.Join(s.dir, filepath.Base(name)))
	if err != nil {
		return Header{}, err
	}
	defer func() { _ = file.Close() }()
	line := make([]byte, headerWidth)
	if _, readErr := io.ReadFull(file, line); readErr != nil {
		return Header{}, readErr
	}
	return decodeHeader(line)
}

func decodeHeader(line []byte) (Header, error) {
	var header Header
	err := json.Unmarshal(bytes.TrimRight(line, " \n"), &header)
	if err != nil {
		return Header{}, err
	}
	return header, nil
}

func (s *Store) Text(id string) (Header, string, error) {
	path, err := s.pathFor(id)
	if err != nil {
		return Header{}, "", err
	}
	file, openErr := os.Open(path)
	if openErr != nil {
		if os.IsNotExist(openErr) {
			return Header{}, "", errUnknownSession
		}
		return Header{}, "", fmt.Errorf("transcript: %w", openErr)
	}
	defer func() { _ = file.Close() }()
	line := make([]byte, headerWidth)
	if _, readErr := io.ReadFull(file, line); readErr != nil {
		return Header{}, "", errUnknownSession
	}
	header, decodeErr := decodeHeader(line)
	if decodeErr != nil {
		return Header{}, "", errUnknownSession
	}
	body, bodyErr := io.ReadAll(io.LimitReader(file, maxBytes+int64(len(truncated))))
	if bodyErr != nil {
		return Header{}, "", fmt.Errorf("transcript: %w", bodyErr)
	}
	return header, string(body), nil
}

func (s *Store) Trim(older time.Duration, now time.Time) (int, error) {
	if !s.On() || older <= 0 {
		return 0, nil
	}
	held, err := s.List(0)
	if err != nil {
		return 0, err
	}
	cutoff := now.Add(-older)
	removed := 0
	for _, header := range held {
		at, parseErr := time.Parse(time.RFC3339, header.At)
		if parseErr != nil || !at.Before(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(s.dir, header.ID+extension)) == nil {
			removed++
		}
	}
	return removed, nil
}

func ErrUnknown() error {
	return errUnknownSession
}
