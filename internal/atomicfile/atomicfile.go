package atomicfile

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const (
	dirMode  = 0o700
	fileMode = 0o600
)

type Saver struct {
	makeDir func(path string, perm os.FileMode) error
	create  func(dir, pattern string) (*os.File, error)
	write   func(file *os.File, body []byte) (int, error)
	sync    func(file *os.File) error
	close   func(file *os.File) error
	chmod   func(path string, mode os.FileMode) error
	rename  func(from, to string) error
	remove  func(path string) error
	syncDir func(path string) error
}

func New() *Saver {
	return &Saver{
		makeDir: os.MkdirAll,
		create:  os.CreateTemp,
		write:   func(file *os.File, body []byte) (int, error) { return file.Write(body) },
		sync:    func(file *os.File) error { return file.Sync() },
		close:   func(file *os.File) error { return file.Close() },
		chmod:   os.Chmod,
		rename:  os.Rename,
		remove:  os.Remove,
		syncDir: syncDirectory,
	}
}

func (s *Saver) Save(path, pattern string, body []byte) error {
	dir := filepath.Dir(path)
	err := s.makeDir(dir, dirMode)
	if err != nil {
		return err
	}
	file, createErr := s.create(dir, pattern)
	if createErr != nil {
		return createErr
	}
	return s.replace(file, path, body)
}

func (s *Saver) replace(file *os.File, path string, body []byte) error {
	temp := file.Name()
	err := s.fill(file, body)
	if err != nil {
		_ = s.remove(temp)
		return err
	}
	chmodErr := s.chmod(temp, fileMode)
	if chmodErr != nil {
		_ = s.remove(temp)
		return chmodErr
	}
	renameErr := s.rename(temp, path)
	if renameErr != nil {
		_ = s.remove(temp)
		return renameErr
	}
	return s.syncDir(filepath.Dir(path))
}

func (s *Saver) fill(file *os.File, body []byte) error {
	written, writeErr := s.write(file, body)
	if writeErr == nil && written != len(body) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = s.sync(file)
	}
	closeErr := s.close(file)
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func Save(path, pattern string, body []byte) error {
	return New().Save(path, pattern, body)
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
