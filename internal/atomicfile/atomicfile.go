// Package atomicfile writes a small file so that a reader never sees half of
// it: the body goes to a temporary file beside the target, is flushed to disk,
// and is then renamed over the target in one step. A failure anywhere leaves
// the previous file exactly as it was and takes the temporary one away.
package atomicfile

import (
	"os"
	"path/filepath"
)

const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Saver is the filesystem as this package uses it. Production wires it to os;
// a test wires it to something that fails where it wants to see what happens.
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

// Save puts body at path, making the directory if it is not there. The pattern
// names the temporary file, which lives beside the target so the rename never
// crosses a filesystem.
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

// fill closes the file whatever happens, because a temporary file that is about
// to be removed still has to be let go of first.
func (s *Saver) fill(file *os.File, body []byte) error {
	defer func() { _ = file.Close() }()
	_, err := s.write(file, body)
	if err != nil {
		return err
	}
	return s.sync(file)
}

// Save writes body at path through the real filesystem.
func Save(path, pattern string, body []byte) error {
	return New().Save(path, pattern, body)
}
