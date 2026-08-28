// Package update asks once per run whether a newer spinoza has been published,
// and hands back the version, a link and the command that installs it.
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

// Endpoint answers in the shape GitHub's own release API answers in.
const Endpoint = "https://spinoza.tech/api/latest"

// Command is the install line the website gives out.
const Command = "curl -fsSL https://spinoza.tech/install.sh | sh"

const (
	askTimeout    = 5 * time.Second
	maxAnswer     = 1 << 20
	releasePrefix = "v"
)

// answer is the part of a release the endpoint publishes that is read here.
type answer struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

// Checker asks once per run.
type Checker struct {
	endpoint string
	current  string
	client   *http.Client

	once   sync.Once
	answer api.UpdateStatus
}

// New takes an endpoint so tests can point elsewhere. Empty means Endpoint.
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

// Status answers from the one request this run makes.
func (c *Checker) Status(ctx context.Context) api.UpdateStatus {
	c.once.Do(func() {
		c.answer = c.ask(ctx)
	})
	return c.answer
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
		status.Command = Command
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

// userAgent is how the endpoint learns which release asked, and on what.
func userAgent(current string) string {
	return "spinoza/" + current + " (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
}

type statusError struct {
	code int
}

func (e *statusError) Error() string {
	return "asking about releases answered " + strconv.Itoa(e.code)
}

// released says whether a version can be compared. A build made outside a
// release carries a commit or the word dev.
func released(version string) bool {
	if !strings.HasPrefix(version, releasePrefix) {
		return false
	}
	return len(parts(version)) == 3
}

// newer orders tags as versions, so that v1.10.0 is above v1.9.0.
func newer(candidate, current string) bool {
	if !released(candidate) {
		return false
	}
	left := parts(candidate)
	right := parts(current)
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

// parts reads the three numbers out of a tag. A pre-release such as v2.0.0-rc.1
// is not one.
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
