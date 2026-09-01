package debugcontainer

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/commandbuffer"
)

const DefaultBinary = "kubectl"

const maxErrorOutput = 64 << 10

type kubectlRunner struct {
	binary     string
	errorLimit int
}

func NewRunner(binary string) Runner {
	if binary == "" {
		binary = DefaultBinary
	}
	return &kubectlRunner{binary: binary, errorLimit: maxErrorOutput}
}

func (k *kubectlRunner) Run(ctx context.Context, args []string) error {
	//nolint:gosec // arguments are validated in Service.admits and passed as argv, never through a shell
	command := exec.CommandContext(ctx, k.binary, args...)
	stderr := commandbuffer.Tail(k.errorLimit)
	command.Stdout = io.Discard
	command.Stderr = stderr
	command.Stdin = nil

	err := command.Run()
	if err == nil {
		return nil
	}

	var missing *exec.Error
	if errors.As(err, &missing) {
		return errors.New("kubectl was not found on PATH; spinoza shells out to it to create debug containers")
	}
	message := meaningful(stderr.String())
	if stderr.Exceeded() {
		return errors.New("kubectl error output exceeded its limit: " + message)
	}
	if message == "" {
		return err
	}
	return errors.New(message)
}

func meaningful(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "--profile=legacy is deprecated") {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "; ")
}
