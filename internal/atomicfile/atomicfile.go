package atomicfile

import (
	"os"
	"path/filepath"
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
	chmod   func(path string, mode os.FileMode) error
	rename  func(from, to string) error
	remove  func(path string) error
}

func New() *Saver {
	return &Saver{
		makeDir: os.MkdirAll,
		create:  os.CreateTemp,
		write:   func(file *os.File, body []byte) (int, error) { return file.Write(body) },
		sync:    func(file *os.File) error { return file.Sync() },
		chmod:   os.Chmod,
		rename:  os.Rename,
		remove:  os.Remove,
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
	return nil
}

func (s *Saver) fill(file *os.File, body []byte) error {
	defer func() { _ = file.Close() }()
	_, err := s.write(file, body)
	if err != nil {
		return err
	}
	return s.sync(file)
}

func Save(path, pattern string, body []byte) error {
	return New().Save(path, pattern, body)
}
