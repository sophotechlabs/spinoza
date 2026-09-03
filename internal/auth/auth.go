package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const flashLifetime = time.Minute

const returnParam = "next"

const flashCookie = "spinoza_login_error"

const (
	maxLogoutTokenBytes       = 16 << 10
	maxBackchannelLogoutBytes = maxLogoutTokenBytes + 1024
)

type Authenticator struct {
	cfg       Config
	sessions  *sessions
	roles     roleMap
	oidc      *provider
	revoked   *revocations
	flows     *consumedFlows
	exchanges *exchangeBudget
	logouts   *logoutVerificationBudget
}

func New(ctx context.Context, cfg Config) (*Authenticator, error) {
	return newAuthenticator(ctx, cfg, rand.Reader)
}

func newAuthenticator(ctx context.Context, cfg Config, entropy io.Reader) (*Authenticator, error) {
	cfg = cfg.withDefaults()
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}
	secret := cfg.SessionSecret
	if len(secret) == 0 {
		secret, err = secretFrom(entropy)
		if err != nil {
			return nil, err
		}
		slog.Warn("no session secret was given, so a new one was generated; every login ends when spinoza restarts")
	}
	auth := &Authenticator{
		cfg:       cfg,
		sessions:  newSessions(secret, cfg.SessionTTL, cfg.SessionMaxAge, secureFor(cfg)),
		roles:     newRoleMap(cfg),
		revoked:   newRevocations(cfg.SessionMaxAge),
		flows:     newConsumedFlows(maxConsumedFlows, time.Now),
		exchanges: newExchangeBudget(8, 4),
		logouts:   newLogoutVerificationBudget(time.Now),
	}
	if cfg.Mode != ModeOIDC {
		return auth, nil
	}
	found, providerErr := newProvider(ctx, cfg.OIDC)
	if providerErr != nil {
		return nil, providerErr
	}
	auth.oidc = found
	announce(found, cfg.OIDC)
	return auth, nil
}

func announce(found *provider, cfg OIDCConfig) {
	if cfg.UnsafeAllowHTTP {
		slog.Warn("oidc plaintext http is enabled; network interception can compromise authentication and the client secret")
	}
	if found.endSession == "" {
		slog.Info("your identity provider advertises no logout endpoint, so signing out here leaves its own session alone")
	}
	if !cfg.BackchannelLogout {
		return
	}
	if !found.backchannel {
		slog.Warn("back-channel logout is on, but your identity provider does not advertise it")
		return
	}
	if !found.sessionIDs {
		slog.Warn("back-channel logout is on, but your identity provider does not put a session id in its logout tokens; spinoza can only end sessions it can name")
	}
}

func secureFor(cfg Config) bool {
	parsed, err := url.Parse(cfg.PublicURL)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https"
}

func (a *Authenticator) Mode() string {
	return a.cfg.Mode
}

func (a *Authenticator) Enabled() bool {
	return a.cfg.Mode != ModeNone
}

func (a *Authenticator) SignsIn() bool {
	return a.oidc != nil
}

func (a *Authenticator) LiveSessionLimit() time.Duration {
	if a == nil || a.cfg.Mode != ModeProxy {
		return 0
	}
	return a.cfg.Proxy.WebSocketMaxAge
}

func (a *Authenticator) Identify(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	switch a.cfg.Mode {
	case ModeNone:
		return Identity{Role: RoleAdmin}, true
	case ModeProxy:
		return a.fromHeaders(r)
	default:
		return a.fromSession(w, r)
	}
}

func (a *Authenticator) StillValid(r *http.Request, expected Identity) bool {
	var current Identity
	var ok bool
	switch a.cfg.Mode {
	case ModeNone:
		current = Identity{Role: RoleAdmin}
		ok = true
	case ModeProxy:
		current, ok = a.fromHeaders(r)
	default:
		held, found := a.sessions.read(r)
		if !found || a.revoked.revoked(held.who.Session) {
			return false
		}
		current = held.who
		ok = true
	}
	return ok && sameIdentity(current, expected)
}

func sameIdentity(left, right Identity) bool {
	if left.User != right.User || left.Role != right.Role || left.Session != right.Session {
		return false
	}
	return slices.Equal(left.Groups, right.Groups)
}

func (a *Authenticator) fromHeaders(r *http.Request) (Identity, bool) {
	if !a.cfg.Proxy.authenticates(r.Header.Get(a.cfg.Proxy.SecretHeader)) {
		return Identity{}, false
	}
	user := strings.TrimSpace(r.Header.Get(a.cfg.Proxy.UserHeader))
	if user == "" {
		return Identity{}, false
	}
	groups := ParseList(r.Header.Get(a.cfg.Proxy.GroupsHeader))
	return Identity{
		User:    user,
		Groups:  groups,
		Role:    a.roles.forGroups(groups),
		Session: user,
	}, true
}

func (a *Authenticator) fromSession(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	held, ok := a.sessions.read(r)
	if !ok {
		return Identity{}, false
	}
	if a.revoked.revoked(held.who.Session) {
		return Identity{}, false
	}
	if a.sessions.halfSpent(held.expires) && a.sessions.renewable(held.issued) {
		renewErr := a.sessions.issue(w, held.who, held.issued)
		if renewErr != nil {
			slog.Warn("a session could not be renewed", "user", held.who.User, "error", renewErr)
		}
	}
	return held.who, true
}

func (a *Authenticator) Login(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		writeAuthError(w, http.StatusNotFound, errNoProvider.Error())
		return
	}
	a.oidc.start(w, r, a.sessions, landingFrom(r))
}

func landingFrom(r *http.Request) string {
	wanted := r.URL.Query().Get(returnParam)
	if !strings.HasPrefix(wanted, "/") {
		return "/"
	}
	if strings.HasPrefix(wanted, "//") {
		return "/"
	}
	if strings.Contains(wanted, `\`) {
		return "/"
	}
	return wanted
}

func (a *Authenticator) Callback(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		writeAuthError(w, http.StatusNotFound, errNoProvider.Error())
		return
	}
	a.sessions.drop(w, flowCookie)
	flow, err := a.oidc.readFlow(r, a.sessions)
	if err != nil {
		a.refuse(w, r, "a login did not complete", err)
		return
	}
	consumeErr := a.flows.consume(flow.State, a.sessions.now().Add(flowLifetime))
	if errors.Is(consumeErr, errFlowRegistryFull) {
		writeAuthError(w, http.StatusTooManyRequests, consumeErr.Error())
		return
	}
	if consumeErr != nil {
		a.refuse(w, r, "a login did not complete", consumeErr)
		return
	}
	if r.URL.Query().Get("code") == "" {
		a.refuse(w, r, "a login did not complete", refusedBy(r))
		return
	}
	release, claimed := a.exchanges.claim(callbackSource(r))
	if !claimed {
		writeAuthError(w, http.StatusTooManyRequests, "login exchange is busy; try again")
		return
	}
	defer release()
	claims, back, err := a.oidc.exchange(r.Context(), r, flow)
	if err != nil {
		a.refuse(w, r, "a login did not complete", err)
		return
	}
	who, identityErr := a.identityFrom(claims)
	if identityErr != nil {
		a.refuse(w, r, "a login carried no usable identity", identityErr)
		return
	}
	issueErr := a.sessions.issue(w, who, a.sessions.now())
	if issueErr != nil {
		writeAuthError(w, http.StatusInternalServerError, issueErr.Error())
		return
	}
	slog.Info("somebody signed in", "user", who.User, "role", who.Role, "groups", len(who.Groups))
	//nolint:gosec // landingFrom keeps this to a path on spinoza itself
	http.Redirect(w, r, back, http.StatusFound)
}

func (a *Authenticator) refuse(w http.ResponseWriter, r *http.Request, what string, err error) {
	slog.Warn(what, "error", err)
	stashErr := a.sessions.stash(w, flashCookie, err.Error(), flashLifetime)
	if stashErr != nil {
		slog.Warn("the reason a login failed could not be kept for the sign-in page", "error", stashErr)
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Authenticator) WhyNot(w http.ResponseWriter, r *http.Request) string {
	var held string
	if !a.sessions.unstashNamed(r, flashCookie, &held) {
		return ""
	}
	a.sessions.drop(w, flashCookie)
	return held
}

func (a *Authenticator) identityFrom(claims claimSet) (Identity, error) {
	name := usernameFrom(claims, a.cfg.OIDC.UsernameClaims)
	if name == "" {
		return Identity{}, errNoUsername
	}
	groups := prefixed(groupsFrom(claims, a.cfg.OIDC.GroupsClaim), a.cfg.OIDC.GroupsPrefix)
	return Identity{
		User:    a.cfg.OIDC.UsernamePrefix + name,
		Groups:  groups,
		Role:    a.roles.forGroups(groups),
		Session: sessionIDFrom(claims),
	}, nil
}

func (a *Authenticator) Logout(w http.ResponseWriter, r *http.Request) {
	who, held := a.Identify(w, r)
	if held && a.oidc != nil {
		a.revoked.revoke(who.Session)
	}
	a.sessions.drop(w, SessionCookie)
	http.Redirect(w, r, a.afterLogout(), http.StatusFound)
}

func (a *Authenticator) afterLogout() string {
	if a.cfg.Mode == ModeProxy && a.cfg.Proxy.LogoutURL != "" {
		return a.cfg.Proxy.LogoutURL
	}
	if a.oidc == nil {
		return "/"
	}
	away := a.oidc.logoutURL()
	if away == "" {
		return "/?prompt=login"
	}
	return away
}

func (a *Authenticator) BackchannelLogout(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil || !a.cfg.OIDC.BackchannelLogout {
		writeAuthError(w, http.StatusNotFound, "back-channel logout is off")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBackchannelLogoutBytes)
	parseErr := r.ParseForm()
	if parseErr != nil {
		var tooBig *http.MaxBytesError
		if errors.As(parseErr, &tooBig) {
			writeAuthError(w, http.StatusRequestEntityTooLarge, "the back-channel logout request is too large")
			return
		}
		writeAuthError(w, http.StatusBadRequest, "the back-channel logout request could not be read")
		return
	}
	raw := r.Form.Get("logout_token")
	if raw == "" {
		writeAuthError(w, http.StatusBadRequest, "the request carried no logout_token")
		return
	}
	if len(raw) > maxLogoutTokenBytes {
		writeAuthError(w, http.StatusRequestEntityTooLarge, "the logout token is too large")
		return
	}
	release, claimed := a.logouts.claim(callbackSource(r))
	if !claimed {
		w.Header().Set("Retry-After", "5")
		writeAuthError(w, http.StatusTooManyRequests, "back-channel logout verification is busy; try again")
		return
	}
	defer release()
	claims, err := a.oidc.verifyLogoutToken(r.Context(), raw)
	if err != nil {
		a.logouts.failed()
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	if claims.SID == "" {
		slog.Warn("a back-channel logout named only a subject, which spinoza cannot map to one session", "subject", claims.Sub)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		return
	}
	a.revoked.revoke(claims.SID)
	slog.Info("a session was ended by the identity provider", "session", claims.SID)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}

type failure struct {
	Message string `json:"message"`
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(failure{Message: message})
	if err != nil {
		slog.Warn("an auth error could not be encoded", "error", err)
	}
}
