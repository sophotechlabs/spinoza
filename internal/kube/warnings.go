package kube

import (
	"log/slog"
	"slices"
	"sync"
)

const deprecationCode = 299

const warningsRemembered = 64

type WarningSink struct {
	log *slog.Logger

	mu   sync.Mutex
	seen map[string]struct{}
}

func newWarningLogger(log *slog.Logger) *WarningSink {
	return &WarningSink{log: log, seen: map[string]struct{}{}}
}

func (w *WarningSink) HandleWarningHeader(code int, agent, text string) {
	if code != deprecationCode || text == "" {
		return
	}
	if !w.first(text) {
		return
	}
	w.log.Warn("the apiserver answered with a warning", "warning", text)
}

func (w *WarningSink) first(text string) bool {
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

func (w *WarningSink) Seen() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.seen))
	for text := range w.seen {
		out = append(out, text)
	}
	slices.Sort(out)
	return out
}
