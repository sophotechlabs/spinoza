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
	full bool
}

func newWarningLogger(log *slog.Logger) *WarningSink {
	return &WarningSink{log: log, seen: map[string]struct{}{}}
}

func (w *WarningSink) HandleWarningHeader(code int, agent, text string) {
	if code != deprecationCode || text == "" {
		return
	}
	first, full := w.remember(text)
	if full {
		w.log.Warn("additional apiserver warnings were omitted")
	}
	if !first {
		return
	}
	w.log.Warn("the apiserver answered with a warning", "warning", text)
}

func (w *WarningSink) remember(text string) (bool, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, repeated := w.seen[text]
	if repeated {
		return false, false
	}
	if len(w.seen) >= warningsRemembered {
		if w.full {
			return false, false
		}
		w.full = true
		return false, true
	}
	w.seen[text] = struct{}{}
	return true, false
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
