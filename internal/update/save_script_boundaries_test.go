package update

import (
	"errors"
	"io"
	"os"
	"testing"
)

type fakeTemporaryScript struct {
	name  string
	write func([]byte) (int, error)
	close func() error
}

func (f fakeTemporaryScript) Name() string {
	return f.name
}

func (f fakeTemporaryScript) Write(body []byte) (int, error) {
	return f.write(body)
}

func (f fakeTemporaryScript) Close() error {
	return f.close()
}

func TestSaveScriptRemovesAFileAfterWriteFailure(t *testing.T) {
	want := errors.New("disk full")
	file := fakeTemporaryScript{
		name:  "/tmp/install.sh",
		write: func([]byte) (int, error) { return 0, want },
		close: func() error { return nil },
	}
	removed := ""

	name, err := saveScriptWith(
		[]byte("script"),
		func() (temporaryScript, error) { return file, nil },
		func(string, os.FileMode) error { return nil },
		func(path string) error {
			removed = path
			return errors.New("cleanup failed")
		},
	)

	if name != "" {
		t.Fatalf("name = %q, want no failed script", name)
	}
	if !errors.Is(err, want) {
		t.Fatalf("save error = %v, want %v", err, want)
	}
	if removed != file.name {
		t.Fatalf("removed = %q, want %q", removed, file.name)
	}
}

func TestSaveScriptRejectsAShortWrite(t *testing.T) {
	file := fakeTemporaryScript{
		name:  "/tmp/install.sh",
		write: func(body []byte) (int, error) { return len(body) - 1, nil },
		close: func() error { return nil },
	}
	removed := ""

	name, err := saveScriptWith(
		[]byte("script"),
		func() (temporaryScript, error) { return file, nil },
		func(string, os.FileMode) error { return nil },
		func(path string) error {
			removed = path
			return nil
		},
	)

	if name != "" {
		t.Fatalf("name = %q, want no truncated script", name)
	}
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("save error = %v, want short write", err)
	}
	if removed != file.name {
		t.Fatalf("removed = %q, want %q", removed, file.name)
	}
}

func TestSaveScriptRemovesAFileAfterCloseFailure(t *testing.T) {
	want := errors.New("close failed")
	file := fakeTemporaryScript{
		name:  "/tmp/install.sh",
		write: func(body []byte) (int, error) { return len(body), nil },
		close: func() error { return want },
	}
	removed := ""

	name, err := saveScriptWith(
		[]byte("script"),
		func() (temporaryScript, error) { return file, nil },
		func(string, os.FileMode) error { return nil },
		func(path string) error {
			removed = path
			return nil
		},
	)

	if name != "" {
		t.Fatalf("name = %q, want no unclosed script", name)
	}
	if !errors.Is(err, want) {
		t.Fatalf("save error = %v, want %v", err, want)
	}
	if removed != file.name {
		t.Fatalf("removed = %q, want %q", removed, file.name)
	}
}

func TestSaveScriptRemovesAFileAfterModeFailure(t *testing.T) {
	want := errors.New("chmod failed")
	file := fakeTemporaryScript{
		name:  "/tmp/install.sh",
		write: func(body []byte) (int, error) { return len(body), nil },
		close: func() error { return nil },
	}
	removed := ""

	name, err := saveScriptWith(
		[]byte("script"),
		func() (temporaryScript, error) { return file, nil },
		func(string, os.FileMode) error { return want },
		func(path string) error {
			removed = path
			return nil
		},
	)

	if name != "" {
		t.Fatalf("name = %q, want no insecure script", name)
	}
	if !errors.Is(err, want) {
		t.Fatalf("save error = %v, want %v", err, want)
	}
	if removed != file.name {
		t.Fatalf("removed = %q, want %q", removed, file.name)
	}
}
