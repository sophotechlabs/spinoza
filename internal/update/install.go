package update

import (
	"bytes"
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

const Script = "https://spinoza.tech/install.sh"

const skipApp = "SPINOZA_SKIP_APP"

const (
	fetchTimeout   = 30 * time.Second
	installTimeout = 5 * time.Minute
	maxScript      = 1 << 20
	scriptMode     = 0o700
)

var installable = map[string]bool{"darwin": true, "linux": true}

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

var ErrUnsupported = errors.New("this build cannot replace itself")

var ErrBusy = errors.New("an update is already running")

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
	if !bytes.Contains(body, []byte(skipApp)) {
		return fmt.Errorf("%w: the install script does not take %s", ErrUnsupported, skipApp)
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

func (i *Installer) directory() (string, error) {
	path, err := i.locate()
	if err != nil {
		return "", fmt.Errorf("finding this binary: %w", err)
	}
	return filepath.Dir(resolve(path)), nil
}

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
	body, err := io.ReadAll(io.LimitReader(response.Body, maxScript+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxScript {
		return nil, fmt.Errorf("install script is larger than %d bytes", maxScript)
	}
	return body, nil
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

func runScript(ctx context.Context, script, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", script)
	cmd.Env = append(
		os.Environ(),
		"SPINOZA_INSTALL_DIR="+dir,
		skipApp+"=1",
	)
	return cmd.CombinedOutput()
}

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
