package kube

import (
	"log/slog"
	"sync"
)

const deprecationCode = 299

const warningsRemembered = 64

type warningLogger struct {
	log *slog.Logger

	mu   sync.Mutex
	seen map[string]struct{}
}

func newWarningLogger(log *slog.Logger) *warningLogger {
	return &warningLogger{log: log, seen: map[string]struct{}{}}
}

func (w *warningLogger) HandleWarningHeader(code int, agent, text string) {
	if code != deprecationCode || text == "" {
		return
	}
	if !w.first(text) {
		return
	}
	w.log.Warn("the apiserver answered with a warning", "warning", text)
}

func (w *warningLogger) first(text string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, repeated := w.seen[text]
	if repeated {
		return false
	}
	if len(w.seen) < warningsRemembered {
		w.seen[text] = struct{}{}
	}
	return true
}
