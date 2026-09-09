package telemetry

import "net/http"

const (
	callOK           = "ok"
	callUnauthorized = "unauthorized"
	callRefused      = "refused"
	callNotFound     = "notFound"
	callConflict     = "conflict"
	callThrottled    = "throttled"
	callServerError  = "serverError"
	callFailed       = "failed"
)

type counted struct {
	inner http.RoundTripper
}

func CountCalls(inner http.RoundTripper) http.RoundTripper {
	return counted{inner: inner}
}

func (c counted) RoundTrip(r *http.Request) (*http.Response, error) {
	answer, err := c.inner.RoundTrip(r)
	if err != nil {
		Default().APIServerCalls.Add(1, callFailed)
		return answer, err
	}
	Default().APIServerCalls.Add(1, outcomeOf(answer.StatusCode))
	return answer, nil
}

func outcomeOf(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return callUnauthorized
	case http.StatusForbidden:
		return callRefused
	case http.StatusNotFound:
		return callNotFound
	case http.StatusConflict:
		return callConflict
	case http.StatusTooManyRequests:
		return callThrottled
	}
	if status >= http.StatusInternalServerError {
		return callServerError
	}
	return callOK
}
