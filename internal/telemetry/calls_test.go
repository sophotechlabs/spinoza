package telemetry

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type answering struct {
	status int
	err    error
}

func (a answering) RoundTrip(_ *http.Request) (*http.Response, error) {
	if a.err != nil {
		return nil, a.err
	}
	return &http.Response{StatusCode: a.status, Body: http.NoBody}, nil
}

func TestAnApiserverCallIsCountedByWhatCameBack(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{name: "it worked", status: http.StatusOK, want: callOK},
		{name: "it was created", status: http.StatusCreated, want: callOK},
		{name: "nobody was signed in", status: http.StatusUnauthorized, want: callUnauthorized},
		{name: "rbac said no", status: http.StatusForbidden, want: callRefused},
		{name: "it was not there", status: http.StatusNotFound, want: callNotFound},
		{name: "something else had it", status: http.StatusConflict, want: callConflict},
		{name: "the apiserver was busy", status: http.StatusTooManyRequests, want: callThrottled},
		{name: "the apiserver broke", status: http.StatusInternalServerError, want: callServerError},
		{name: "a gateway broke", status: http.StatusBadGateway, want: callServerError},
		{name: "the request was wrong", status: http.StatusBadRequest, want: callOK},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			if got := outcomeOf(one.status); got != one.want {
				t.Fatalf("outcome = %q, want %q", got, one.want)
			}
		})
	}
}

func TestTheWrapperCountsEveryCallItPassesThrough(t *testing.T) {
	before := page(t)

	inner := CountCalls(answering{status: http.StatusForbidden})
	answer, err := inner.RoundTrip(httptest.NewRequest(http.MethodGet, "/api", http.NoBody))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if answer.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want the one the inner transport gave", answer.StatusCode)
	}
	if !moved(before, page(t), `spinoza_apiserver_calls_total{outcome="refused"}`) {
		t.Fatal("a refused call was not counted")
	}
}

func TestACallThatNeverLandedIsCountedAsFailed(t *testing.T) {
	before := page(t)
	wanted := errors.New("no route to host")

	inner := CountCalls(answering{err: wanted})
	_, err := inner.RoundTrip(httptest.NewRequest(http.MethodGet, "/api", http.NoBody))

	if !errors.Is(err, wanted) {
		t.Fatalf("error = %v, want the one the transport gave", err)
	}
	if !moved(before, page(t), `spinoza_apiserver_calls_total{outcome="failed"}`) {
		t.Fatal("a call that never landed was not counted")
	}
}

func page(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	if err := Default().Registry.Write(&out); err != nil {
		t.Fatalf("write: %v", err)
	}
	return out.String()
}

func moved(before, after, series string) bool {
	return countedIn(after, series) > countedIn(before, series)
}

func countedIn(body, series string) int {
	for line := range strings.SplitSeq(body, "\n") {
		name, value, found := strings.Cut(line, " ")
		if !found || name != series {
			continue
		}
		total := 0
		for _, letter := range value {
			if letter < '0' || letter > '9' {
				break
			}
			total = total*10 + int(letter-'0')
		}
		return total
	}
	return 0
}
