// Package update asks once, per run, whether a newer spinoza has been
// published, and says so.
//
// It never installs anything. What it hands back is a version number, a link
// and the command that would install it, which is the same command the website
// gives out. Deciding to run that is the person's, not spinoza's.
package update

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// Endpoint is asked what the newest release is. GitHub's own answer is the
// shape read here, so anything that repeats that shape can stand in front of it
// — which is how spinoza.tech will, once it is serving one.
const Endpoint = "https://api.github.com/repos/sophotechlabs/spinoza/releases/latest"

// Command is how a person installs the new one, word for word what the website
// tells them. Spinoza only ever shows it.
const Command = "curl -fsSL https://spinoza.tech/install.sh | sh"

const (
	askTimeout = 5 * time.Second
	maxAnswer  = 1 << 20
	// A build that is not a release has nothing to compare against: what it
	// carries is a commit, and no commit is newer or older than v1.2.3.
	releasePrefix = "v"
)

// answer is the part of a release GitHub publishes that matters here.
type answer struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

// Checker asks the once per run that the answer is worth. A version does not
// change while spinoza is open, and a tool that asks the internet on a timer is
// a tool doing something its user did not ask for.
type Checker struct {
	endpoint string
	current  string
	client   *http.Client

	once   sync.Once
	answer api.UpdateStatus
}

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

// Off is a checker that answers without asking anybody, for a run started with
// the check turned off.
func Off(current string) *Checker {
	checker := &Checker{current: current}
	checker.once.Do(func() {
		checker.answer = api.UpdateStatus{
			Current: current,
			Reason:  "spinoza was started with --update-check=false",
		}
	})
	return checker
}

// Status is what spinoza knows about newer releases. The first caller pays for
// the question and everyone after reads the answer.
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return answer{}, err
	}
	request.Header.Set("Accept", "application/json")
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

type statusError struct {
	code int
}

func (e *statusError) Error() string {
	return "asking about releases answered " + strconv.Itoa(e.code)
}

// released says whether a version is one that can be compared. A build made
// outside a release carries a commit or the word dev.
func released(version string) bool {
	if !strings.HasPrefix(version, releasePrefix) {
		return false
	}
	return len(parts(version)) == 3
}

// newer compares two release tags the way versions are ordered, so that v1.10.0
// is above v1.9.0 rather than below it the way strings would have it.
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

// parts reads the three numbers out of a tag, and nothing out of anything else.
// A pre-release such as v2.0.0-rc.1 is deliberately not one of them: spinoza
// does not offer a person a release candidate.
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
