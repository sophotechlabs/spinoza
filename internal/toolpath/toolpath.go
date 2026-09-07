package toolpath

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/commandbuffer"
)

const probeTimeout = 10 * time.Second

const pipeGrace = time.Second

const maxEnvironmentBytes = 1 << 20

const marker = "spinoza-environment"

var systemDirs = []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}

var sessionOnly = []string{"PATH", "_", "SHLVL", "PWD", "OLDPWD"}

var setenv = os.Setenv

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

func FromLoginShell(ctx context.Context, shell string) (map[string]string, error) {
	if shell == "" {
		return nil, errors.New("there is no login shell to ask")
	}
	asking, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out := commandbuffer.Head(maxEnvironmentBytes)
	script := fmt.Sprintf(`printf '\n%s\n'; env -0 2>/dev/null || env`, marker)
	command := exec.CommandContext(asking, shell, "-l", "-i", "-c", script)
	command.Stdout = out
	command.WaitDelay = pipeGrace
	err := command.Run()
	if err != nil {
		return nil, fmt.Errorf("asking %s for its environment: %w", shell, err)
	}
	if out.Exceeded() {
		return nil, fmt.Errorf("%s reported an environment larger than %d bytes", shell, maxEnvironmentBytes)
	}
	found := parse(out.String())
	if found["PATH"] == "" {
		return nil, fmt.Errorf("%s reported no PATH", shell)
	}
	return found, nil
}

func parse(raw string) map[string]string {
	_, after, found := strings.Cut(raw, "\n"+marker+"\n")
	if !found {
		return nil
	}
	held := map[string]string{}
	for _, entry := range entries(after) {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if !identifier(key) {
			continue
		}
		held[key] = value
	}
	return held
}

func entries(raw string) []string {
	if strings.Contains(raw, "\x00") {
		return strings.Split(raw, "\x00")
	}
	joined := []string{}
	for line := range strings.SplitSeq(strings.TrimSuffix(raw, "\n"), "\n") {
		key, _, ok := strings.Cut(line, "=")
		if ok && identifier(key) {
			joined = append(joined, line)
			continue
		}
		if len(joined) == 0 {
			continue
		}
		joined[len(joined)-1] += "\n" + line
	}
	return joined
}

func identifier(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func Ensure(ctx context.Context, shell string) string {
	current := os.Getenv("PATH")
	if !Bare(current) {
		return current
	}
	found, err := FromLoginShell(ctx, shell)
	if err != nil {
		slog.Warn("tools and settings from the login shell will not be found", "error", err)
		return current
	}
	merged := Merge(current, found["PATH"])
	setErr := setenv("PATH", merged)
	if setErr != nil {
		slog.Warn("tools outside the system directories will not be found", "error", setErr)
		return current
	}
	added := adopt(found)
	slog.Info("took the environment from the login shell", "shell", shell, "added", added)
	return merged
}

func adopt(found map[string]string) []string {
	added := []string{}
	for _, key := range slices.Sorted(maps.Keys(found)) {
		if slices.Contains(sessionOnly, key) {
			continue
		}
		_, set := os.LookupEnv(key)
		if set {
			continue
		}
		err := setenv(key, found[key])
		if err != nil {
			slog.Warn("a login shell variable was not taken", "name", key, "error", err)
			continue
		}
		added = append(added, key)
	}
	return added
}
