package toolpath

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/commandbuffer"
)

const probeTimeout = 5 * time.Second

const maxPathBytes = 64 * 1024

var systemDirs = []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}

func Bare(path string) bool {
	for dir := range strings.SplitSeq(path, ":") {
		if dir == "" {
			continue
		}
		if !slices.Contains(systemDirs, dir) {
			return false
		}
	}
	return true
}

func Merge(current, found string) string {
	seen := map[string]bool{}
	kept := []string{}
	for _, dir := range append(strings.Split(current, ":"), strings.Split(found, ":")...) {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		kept = append(kept, dir)
	}
	return strings.Join(kept, ":")
}

func FromLoginShell(ctx context.Context, shell string) (string, error) {
	if shell == "" {
		return "", errors.New("there is no login shell to ask")
	}
	asking, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out := commandbuffer.Head(maxPathBytes)
	command := exec.CommandContext(asking, shell, "-l", "-c", `printf %s "$PATH"`)
	command.Stdout = out
	err := command.Run()
	if err != nil {
		return "", fmt.Errorf("asking %s for its PATH: %w", shell, err)
	}
	if out.Exceeded() {
		return "", fmt.Errorf("%s reported a PATH larger than %d bytes", shell, maxPathBytes)
	}
	found := strings.TrimSpace(out.String())
	if found == "" {
		return "", fmt.Errorf("%s reported no PATH", shell)
	}
	return found, nil
}

func Ensure(ctx context.Context, shell string) string {
	current := os.Getenv("PATH")
	if !Bare(current) {
		return current
	}
	found, err := FromLoginShell(ctx, shell)
	if err != nil {
		slog.Warn("tools outside the system directories will not be found", "error", err)
		return current
	}
	merged := Merge(current, found)
	setErr := os.Setenv("PATH", merged)
	if setErr != nil {
		slog.Warn("tools outside the system directories will not be found", "error", setErr)
		return current
	}
	slog.Info("took the PATH from the login shell", "shell", shell)
	return merged
}
