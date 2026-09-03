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
	"strings"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/commandbuffer"
)

const Script = "https://spinoza.tech/install.sh"

const skipApp = "SPINOZA_SKIP_APP"

const (
	fetchTimeout   = 30 * time.Second
	installTimeout = 5 * time.Minute
	maxScript      = 1 << 20
	maxRunOutput   = 64 << 10
	scriptMode     = 0o700
	maxRedirects   = 10
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
	installer := &Installer{
		current:  current,
		script:   script,
		goos:     runtime.GOOS,
		locate:   os.Executable,
		run:      runScript,
		writable: writableDir,
	}
	installer.client = &http.Client{
		Timeout:       fetchTimeout,
		CheckRedirect: installer.checkRedirect,
	}
	return installer
}

var ErrUnsupported = errors.New("this build cannot replace itself")

var ErrBusy = errors.New("an update is already running")

func (i *Installer) Install(ctx context.Context) error {
	if i.script == "" {
		return fmt.Errorf("%w: remote installer scripts are disabled by default", ErrUnsupported)
	}
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

func (i *Installer) checkRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("install script had too many redirects")
	}
	if len(via) == 0 {
		return nil
	}
	origin := via[0].URL
	if request.URL.Scheme != origin.Scheme {
		return errors.New("install script redirect changed origin")
	}
	if request.URL.Host != origin.Host {
		return errors.New("install script redirect changed origin")
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
	create := func() (temporaryScript, error) {
		return os.CreateTemp("", "spinoza-install-*.sh")
	}
	return saveScriptWith(body, create, os.Chmod, os.Remove)
}

type temporaryScript interface {
	Name() string
	Write(body []byte) (int, error)
	Close() error
}

func saveScriptWith(
	body []byte,
	create func() (temporaryScript, error),
	chmod func(string, os.FileMode) error,
	remove func(string) error,
) (string, error) {
	file, err := create()
	if err != nil {
		return "", err
	}
	name := file.Name()
	written, writeErr := file.Write(body)
	closeErr := file.Close()
	if writeErr != nil {
		_ = remove(name)
		return "", writeErr
	}
	if written != len(body) {
		_ = remove(name)
		return "", io.ErrShortWrite
	}
	if closeErr != nil {
		_ = remove(name)
		return "", closeErr
	}
	if chmodErr := chmod(name, scriptMode); chmodErr != nil {
		_ = remove(name)
		return "", chmodErr
	}
	return name, nil
}

func runScript(ctx context.Context, script, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", script)
	cmd.Env = append(
		installerEnvironment(),
		"SPINOZA_INSTALL_DIR="+dir,
		skipApp+"=1",
	)
	output := commandbuffer.Tail(maxRunOutput)
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	if output.Exceeded() {
		if err != nil {
			return output.Bytes(), fmt.Errorf("install output exceeded %d bytes: %w", maxRunOutput, err)
		}
		return output.Bytes(), fmt.Errorf("install output exceeded %d bytes", maxRunOutput)
	}
	return output.Bytes(), err
}

func installerEnvironment() []string {
	allowed := map[string]bool{
		"HOME":          true,
		"LANG":          true,
		"PATH":          true,
		"SHELL":         true,
		"TEMP":          true,
		"TMP":           true,
		"TMPDIR":        true,
		"HTTP_PROXY":    true,
		"HTTPS_PROXY":   true,
		"NO_PROXY":      true,
		"http_proxy":    true,
		"https_proxy":   true,
		"no_proxy":      true,
		"SSL_CERT_DIR":  true,
		"SSL_CERT_FILE": true,
	}
	result := make([]string, 0, len(allowed))
	for _, value := range os.Environ() {
		name, _, found := strings.Cut(value, "=")
		if !found {
			continue
		}
		if !allowedInstallerVariable(name, allowed) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func allowedInstallerVariable(name string, allowed map[string]bool) bool {
	if allowed[name] {
		return true
	}
	if strings.HasPrefix(name, "LC_") {
		return true
	}
	return strings.HasPrefix(name, "XDG_")
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
