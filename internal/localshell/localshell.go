package localshell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

const fallbackShell = "/bin/sh"

const preferredShell = "/bin/zsh"

type Size struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type Options struct {
	Shell string
	Dir   string
	Env   []string
	Size  Size
}

func command(ctx context.Context, shell string) *exec.Cmd {
	return exec.CommandContext(ctx, shell, "-l")
}

type Session struct {
	tty     *os.File
	cmd     *exec.Cmd
	done    chan error
	once    sync.Once
	closing sync.Mutex
}

func readable(path string) bool {
	//nolint:gosec // the path is the shell this user already runs, from their own environment
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func ShellPath() string {
	chosen := os.Getenv("SHELL")
	if chosen != "" && readable(chosen) {
		return chosen
	}
	if readable(preferredShell) {
		return preferredShell
	}
	return fallbackShell
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return home
}

func environment(extra []string) []string {
	env := os.Environ()
	env = append(env, "TERM=xterm-256color")
	return append(env, extra...)
}

func (o Options) orDefaults() Options {
	if o.Shell == "" {
		o.Shell = ShellPath()
	}
	if o.Dir == "" {
		o.Dir = homeDir()
	}
	if o.Size.Cols == 0 {
		o.Size.Cols = 80
	}
	if o.Size.Rows == 0 {
		o.Size.Rows = 24
	}
	return o
}

func Start(ctx context.Context, opts Options) (*Session, error) {
	opts = opts.orDefaults()
	cmd := command(ctx, opts.Shell)
	cmd.Dir = opts.Dir
	cmd.Env = environment(opts.Env)

	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: opts.Size.Cols, Rows: opts.Size.Rows})
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", opts.Shell, err)
	}

	session := &Session{tty: tty, cmd: cmd, done: make(chan error, 1)}
	go session.wait()
	return session, nil
}

func (s *Session) wait() {
	err := s.cmd.Wait()
	s.closing.Lock()
	_ = s.tty.Close()
	s.closing.Unlock()
	s.done <- exitError(err)
}

func exitError(err error) error {
	if err == nil {
		return nil
	}
	if _, exit := errors.AsType[*exec.ExitError](err); exit {
		return nil
	}
	return err
}

func (s *Session) Read(p []byte) (int, error) {
	read, err := s.tty.Read(p)
	if errors.Is(err, fs.ErrClosed) {
		return read, io.EOF
	}
	return read, err
}

func (s *Session) Write(p []byte) (int, error) {
	return s.tty.Write(p)
}

func (s *Session) Resize(size Size) {
	if size.Cols == 0 || size.Rows == 0 {
		return
	}
	_ = pty.Setsize(s.tty, &pty.Winsize{Cols: size.Cols, Rows: size.Rows})
}

func (s *Session) Done() <-chan error {
	return s.done
}

func (s *Session) Close() {
	s.once.Do(func() {
		s.closing.Lock()
		_ = s.tty.Close()
		s.closing.Unlock()
		process := s.cmd.Process
		if process != nil {
			_ = process.Kill()
		}
	})
}
