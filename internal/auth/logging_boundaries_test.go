package auth

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func captureAuthLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	original := slog.Default()
	var logged bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() {
		slog.SetDefault(original)
	})
	return &logged
}

func TestBackchannelLogoutWarnsWhenTheProviderDoesNotAdvertiseIt(t *testing.T) {
	logged := captureAuthLogs(t)

	announce(&provider{endSession: "https://idp.example/logout"}, OIDCConfig{BackchannelLogout: true})

	message := logged.String()
	if !strings.Contains(message, "does not advertise it") {
		t.Fatalf("log = %q, want the unavailable back-channel logout named", message)
	}
	if strings.Contains(message, "session id") {
		t.Fatalf("log = %q, want no claim that logout tokens were inspected", message)
	}
}

type failedAuthResponse struct {
	header http.Header
}

func (w *failedAuthResponse) Header() http.Header {
	return w.header
}

func (*failedAuthResponse) WriteHeader(int) {}

func (*failedAuthResponse) Write([]byte) (int, error) {
	return 0, errors.New("connection reset")
}

func TestAnAuthErrorSurvivesTheClientDisconnectingWhileItIsWritten(t *testing.T) {
	logged := captureAuthLogs(t)
	w := &failedAuthResponse{header: http.Header{}}

	writeAuthError(w, http.StatusUnauthorized, "sign in again")

	message := logged.String()
	if !strings.Contains(message, "could not be encoded") || !strings.Contains(message, "connection reset") {
		t.Fatalf("log = %q, want the disconnected response reported", message)
	}
}
