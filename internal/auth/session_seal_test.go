package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestSealRejectsAValueJSONCannotRepresent(t *testing.T) {
	value, err := testSessions(t, false).seal(make(chan int))

	if err == nil {
		t.Fatal("an unsupported value was signed as a session payload")
	}
	if value != "" {
		t.Fatalf("value = %q, want nothing signed after encoding failed", value)
	}
}

func TestSealRejectsAPayloadPastTheCookieLimit(t *testing.T) {
	value, err := testSessions(t, false).seal(strings.Repeat("x", maxCookieBytes))

	if !errors.Is(err, errCookieTooBig) {
		t.Fatalf("error = %v, want the cookie size limit", err)
	}
	if value != "" {
		t.Fatalf("value length = %d, want no oversized cookie value", len(value))
	}
}
