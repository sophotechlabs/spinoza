// Package update reports whether a newer spinoza has been published.
package update

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const Endpoint = "https://spinoza.tech/api/latest"

const Command = "curl -fsSL https://spinoza.tech/install.sh | sh"

const PowerShellCommand = "irm https://spinoza.tech/install.ps1 | iex"

func InstallCommand() string {
	return commandFor(runtime.GOOS)
}

func commandFor(goos string) string {
	if goos == "windows" {
		return PowerShellCommand
	}
	return Command
}

const (
	askTimeout    = 5 * time.Second
	maxAnswer     = 1 << 20
	releasePrefix = "v"
)

type answer struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

type Checker struct {
	endpoint string
	current  string
	client   *http.Client

	mu     sync.Mutex
	asked  bool
	answer api.UpdateStatus
}

// New takes an endpoint for tests; empty means Endpoint.
func New(current, endpoint string) *Checker {
	if endpoint == "" {
		endpoint = Endpoint
	}
	return &Checker{
		endpoint: endpoint,
		current:  current,
		client:   &http.Client{Timeout: askTimeout},
	}
}

func (c *Checker) Status(ctx context.Context) api.UpdateStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.asked {
		return c.answer
	}
	c.answer = c.ask(ctx)
	c.asked = true
	return c.answer
}

// Recheck asks again. Pressing a button is a reason to; opening a window is not.
func (c *Checker) Recheck(ctx context.Context) api.UpdateStatus {
	answer := c.ask(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.answer = answer
	c.asked = true
	return answer
}

func (c *Checker) ask(ctx context.Context) api.UpdateStatus {
	status := api.UpdateStatus{Current: c.current}
	if !released(c.current) {
		status.Reason = "this build was not made from a release, so there is nothing to compare"
		return status
	}
	found, err := c.fetch(ctx)
	if err != nil {
		status.Reason = err.Error()
		return status
	}
	status.Checked = true
	status.Latest = found.Tag
	status.URL = found.URL
	if newer(found.Tag, c.current) {
		status.Available = true
		status.Command = InstallCommand()
	}
	return status
}

func (c *Checker) fetch(ctx context.Context) (answer, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, http.NoBody)
	if err != nil {
		return answer{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent(c.current))
	response, err := c.client.Do(request)
	if err != nil {
		return answer{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return answer{}, &statusError{code: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer))
	if err != nil {
		return answer{}, err
	}
	var found answer
	if unmarshalErr := json.Unmarshal(body, &found); unmarshalErr != nil {
		return answer{}, unmarshalErr
	}
	return found, nil
}

func userAgent(current string) string {
	return "spinoza/" + current + " (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
}

type statusError struct {
	code int
}

func (e *statusError) Error() string {
	return "asking about releases answered " + strconv.Itoa(e.code)
}

func released(version string) bool {
	if !strings.HasPrefix(version, releasePrefix) {
		return false
	}
	return len(parts(version)) == 3
}

// Numeric, so v1.10.0 sorts above v1.9.0.
func newer(candidate, current string) bool {
	if !released(candidate) {
		return false
	}
	left := parts(candidate)
	right := parts(current)
	if len(right) != len(left) {
		return true
	}
	for i := range left {
		if left[i] > right[i] {
			return true
		}
		if left[i] < right[i] {
			return false
		}
	}
	return false
}

func parts(version string) []int {
	trimmed := strings.TrimPrefix(version, releasePrefix)
	fields := strings.Split(trimmed, ".")
	if len(fields) != 3 {
		return nil
	}
	out := make([]int, 0, len(fields))
	for _, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil {
			return nil
		}
		if number < 0 {
			return nil
		}
		out = append(out, number)
	}
	return out
}
