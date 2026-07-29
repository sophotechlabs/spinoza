package debugcontainer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
)

const DefaultBinary = "kubectl"

type kubectlRunner struct {
	binary string
}

func NewRunner(binary string) Runner {
	if binary == "" {
		binary = DefaultBinary
	}
	return &kubectlRunner{binary: binary}
}

func (k *kubectlRunner) Run(ctx context.Context, args []string) error {
	//nolint:gosec // arguments are validated in Service.admits and passed as argv, never through a shell
	command := exec.CommandContext(ctx, k.binary, args...)
	var stderr bytes.Buffer
	command.Stdout = io.Discard
	command.Stderr = &stderr
	command.Stdin = nil

	err := command.Run()
	if err == nil {
		return nil
	}

	var missing *exec.Error
	if errors.As(err, &missing) {
		return errors.New("kubectl was not found on PATH — spinoza shells out to it to create debug containers")
	}
	message := meaningful(stderr.String())
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
