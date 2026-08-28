package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Script is the install script the website serves, and the one the command in
// the toast pipes to a shell.
const Script = "https://spinoza.tech/install.sh"

const (
	fetchTimeout   = 30 * time.Second
	installTimeout = 5 * time.Minute
	maxScript      = 1 << 20
	scriptMode     = 0o700
)

// installable names the systems install.sh knows how to write to. Windows has
// no path through it at all.
var installable = map[string]bool{"darwin": true, "linux": true}

// Installer replaces the running binary by fetching install.sh and running it
// against the directory that binary sits in.
//
// A desktop build wires none: replacing an app bundle while it is running is
// not something to attempt from inside it.
type Installer struct {
	current  string
	script   string
	client   *http.Client
	goos     string
	locate   func() (string, error)
	run      func(ctx context.Context, script, dir string) ([]byte, error)
	writable func(dir string) error

	mu      sync.Mutex
	running bool
}

func NewInstaller(current, script string) *Installer {
	if script == "" {
		script = Script
	}
	return &Installer{
		current:  current,
		script:   script,
		client:   &http.Client{Timeout: fetchTimeout},
		goos:     runtime.GOOS,
		locate:   os.Executable,
		run:      runScript,
		writable: writableDir,
	}
}

// ErrUnsupported is what a system install.sh cannot write to earns.
var ErrUnsupported = errors.New("this build cannot replace itself")

// ErrBusy is a second press while the first is still running.
var ErrBusy = errors.New("an update is already running")

// Install fetches the script and runs it against the directory holding this
// binary, so it replaces this one rather than whichever the script would pick.
// The desktop app is never installed alongside it.
func (i *Installer) Install(ctx context.Context) error {
	if !installable[i.goos] {
		return fmt.Errorf("%w: %s has no install script", ErrUnsupported, i.goos)
	}
	dir, err := i.directory()
	if err != nil {
		return err
	}
	if writeErr := i.writable(dir); writeErr != nil {
		return fmt.Errorf("%w: %w", ErrUnsupported, writeErr)
	}
	if !i.claim() {
		return ErrBusy
	}
	defer i.release()
	body, fetchErr := i.fetch(ctx)
	if fetchErr != nil {
		return fetchErr
	}
	path, saveErr := saveScript(body)
	if saveErr != nil {
		return saveErr
	}
	defer func() { _ = os.Remove(path) }()
	bounded, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	output, runErr := i.run(bounded, path, dir)
	if runErr != nil {
		return fmt.Errorf("%w: %s", runErr, lastLine(output))
	}
	return nil
}

// directory is where this binary lives, with symlinks resolved so that a linked
// name keeps pointing at the file that was replaced.
func (i *Installer) directory() (string, error) {
	path, err := i.locate()
	if err != nil {
		return "", fmt.Errorf("finding this binary: %w", err)
	}
	return filepath.Dir(resolve(path)), nil
}

// resolve follows a symlinked name to the file it points at. One that cannot be
// followed is used as it is.
func resolve(path string) string {
	linked, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return linked
}

func (i *Installer) claim() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.running {
		return false
	}
	i.running = true
	return true
}

func (i *Installer) release() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.running = false
}

func (i *Installer) fetch(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, i.script, http.NoBody)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent(i.current))
	response, err := i.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, &statusError{code: response.StatusCode}
	}
	return io.ReadAll(io.LimitReader(response.Body, maxScript))
}

func saveScript(body []byte) (string, error) {
	file, err := os.CreateTemp("", "spinoza-install-*.sh")
	if err != nil {
		return "", err
	}
	name := file.Name()
	_, writeErr := file.Write(body)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(name)
		return "", writeErr
	}
	if closeErr != nil {
		_ = os.Remove(name)
		return "", closeErr
	}
	if chmodErr := os.Chmod(name, scriptMode); chmodErr != nil {
		_ = os.Remove(name)
		return "", chmodErr
	}
	return name, nil
}

// runScript runs the saved script with sh. The directory and the skip travel as
// environment rather than as a shell line, so a path with a space or a quote in
// it is a path and not a second command.
func runScript(ctx context.Context, script, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", script)
	cmd.Env = append(
		os.Environ(),
		"SPINOZA_INSTALL_DIR="+dir,
		"SPINOZA_SKIP_APP=1",
	)
	return cmd.CombinedOutput()
}

// writableDir reports what stops a write here, which is how a binary in
// /usr/local/bin owned by root is told apart from one in a home directory.
func writableDir(dir string) error {
	file, err := os.CreateTemp(dir, ".spinoza-write-*")
	if err != nil {
		return fmt.Errorf("%s is not writable", dir)
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return nil
}

func lastLine(output []byte) string {
	text := string(output)
	for text != "" && (text[len(text)-1] == '\n' || text[len(text)-1] == '\r') {
		text = text[:len(text)-1]
	}
	if text == "" {
		return "no output"
	}
	for at := len(text) - 1; at >= 0; at-- {
		if text[at] == '\n' {
			return text[at+1:]
		}
	}
	return text
}
