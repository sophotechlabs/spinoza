package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/store"
)

const (
	AuditSignedIn       = "signed in"
	AuditSignedOut      = "signed out"
	AuditSessionExpired = "session expired"
	AuditRoleRefused    = "role refused"
	AuditActingRefused  = "impersonation refused"
)

func (s *Server) RecordIdentity(r *http.Request, verb string, who auth.Identity) {
	s.auditIdentity(r, verb, who, api.HistoryDone, "")
}

func (s *Server) RecordIdentityRefused(r *http.Request, verb string, who auth.Identity, why string) {
	s.auditIdentity(r, verb, who, api.HistoryRefused, why)
}

func (s *Server) auditIdentity(r *http.Request, verb string, who auth.Identity, outcome, why string) {
	past := s.recorder()
	if past == nil {
		return
	}
	kept, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), recordTimeout)
	defer cancel()
	_, on := s.lookup(clusterOf(r))
	err := past.For(on).Record(kept, store.Entry{
		At:      s.instant(),
		Verb:    verb,
		Actor:   auditActor(who),
		Outcome: outcome,
		Message: auditText(why),
	})
	if err != nil {
		slog.Warn("who was here was not recorded", "verb", verb, "error", err)
		return
	}
	s.auditRecorded(kept, past)
}

func auditActor(who auth.Identity) string {
	if who.User == "" {
		return "anonymous"
	}
	return who.User
}
