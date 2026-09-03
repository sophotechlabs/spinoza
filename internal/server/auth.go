package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

const (
	pathLogin       = "/auth/login"
	pathCallback    = "/auth/callback"
	pathLogout      = "/auth/logout"
	pathBackchannel = "/auth/backchannel-logout"
	pathSession     = "/api/auth/me"
	pathIndex       = "/index.html"
	needsSigningIn  = "spinoza needs you to sign in"
	noSuchOrigin    = "spinoza answers pages served from its own address only"
)

type ClusterAuth struct {
	Authenticator *auth.Authenticator
	PublicURL     string
}

func (s *Server) UseClusterAuth(settings ClusterAuth) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authn = settings.Authenticator
	s.publicOrigin = originOfURL(settings.PublicURL)
	s.served = true
}

func originOfURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func (s *Server) inCluster() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.served
}

func (s *Server) authenticator() *auth.Authenticator {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authn
}

func (s *Server) identify(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	authn := s.authenticator()
	if authn == nil {
		return auth.Identity{Role: auth.RoleAdmin}, true
	}
	return authn.Identify(w, r)
}

func (s *Server) expectedOrigin() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publicOrigin
}

func (s *Server) sessionRoute() endpoint {
	return endpoint{http.MethodGet, pathSession, s.handleSession, true, false}
}

func (s *Server) signInRoutes() []endpoint {
	return []endpoint{
		{http.MethodGet, pathLogin, s.handleLogin, true, false},
		{http.MethodGet, pathCallback, s.handleCallback, true, false},
		{http.MethodPost, pathLogout, s.handleLogout, true, false},
		{http.MethodPost, pathBackchannel, s.handleBackchannelLogout, true, false},
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	s.authenticator().Login(w, r)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	s.authenticator().Callback(w, r)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.authenticator().Logout(w, r)
}

func (s *Server) handleBackchannelLogout(w http.ResponseWriter, r *http.Request) {
	s.authenticator().BackchannelLogout(w, r)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.sessionFor(w, r))
}

func (s *Server) sessionFor(w http.ResponseWriter, r *http.Request) api.Session {
	authn := s.authenticator()
	if authn == nil {
		return api.Session{Mode: auth.ModeNone, Role: auth.RoleAdmin, Authenticated: true}
	}
	out := api.Session{Mode: authn.Mode(), SignIn: authn.SignsIn(), Cluster: true}
	who, ok := auth.IdentityFrom(r.Context())
	if !ok {
		out.Error = authn.WhyNot(w, r)
		return out
	}
	out.Authenticated = true
	out.User = who.User
	out.Groups = who.Groups
	out.Role = who.Role
	out.Scope = s.scopeOf(r)
	return out
}

func (s *Server) scopeOf(r *http.Request) api.Scope {
	backend := s.managerFor(r)
	if backend == nil {
		return api.Scope{Everywhere: true}
	}
	return backend.Scope(r.Context())
}

func publicWhenServing(r *http.Request) bool {
	if publicAsset(r) {
		return true
	}
	if r.Method == http.MethodPost {
		if r.URL.Path == pathBackchannel {
			return true
		}
		return r.URL.Path == pathLogout
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	switch r.URL.Path {
	case "/", pathIndex, "/healthz", pathSession, pathLogin, pathCallback, pathLogout:
		return true
	default:
		return false
	}
}

func (s *Server) admit(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if s.inCluster() {
		return s.admitServed(w, r)
	}
	return s.admitLocal(w, r)
}

func (s *Server) admitServed(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if !s.ownAddress(r) {
		writeError(w, http.StatusForbidden, noSuchOrigin)
		return nil, false
	}
	who, known := s.identify(w, r)
	if known {
		r = r.WithContext(access.WithScopeSlot(auth.WithIdentity(r.Context(), who)))
	}
	if publicWhenServing(r) {
		return r, true
	}
	if !known {
		writeError(w, http.StatusUnauthorized, needsSigningIn)
		return nil, false
	}
	return r, true
}

func (s *Server) ownAddress(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		if topLevelNavigation(r) {
			return true
		}
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	wanted := s.expectedOrigin()
	if wanted == "" {
		return allowedOrigin(origin, r.Host)
	}
	return strings.EqualFold(originOfURL(origin), wanted)
}
