package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/sophotechlabs/spinoza/internal/filetx"
)

const flowCookie = "spinoza_login"

const flowLifetime = 10 * time.Minute

const providerTimeout = 20 * time.Second

const maxCACertBytes = 4 << 20

type discovered struct {
	EndSession                 string   `json:"end_session_endpoint"`
	BackchannelLogout          bool     `json:"backchannel_logout_supported"`
	BackchannelLogoutSession   bool     `json:"backchannel_logout_session_supported"`
	AuthorizationEndpoint      string   `json:"authorization_endpoint"`
	TokenEndpoint              string   `json:"token_endpoint"`
	UserInfoEndpoint           string   `json:"userinfo_endpoint"`
	JWKSEndpoint               string   `json:"jwks_uri"`
	IDTokenSigningAlgSupported []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported            []string `json:"scopes_supported"`
}

type provider struct {
	cfg         OIDCConfig
	oauth       oauth2.Config
	verifier    *oidc.IDTokenVerifier
	client      *http.Client
	endSession  string
	backchannel bool
	sessionIDs  bool
}

func httpClientFor(cfg OIDCConfig) (*http.Client, error) {
	transport, err := transportFor(cfg)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport, Timeout: providerTimeout}, nil
}

func transportFor(cfg OIDCConfig) (http.RoundTripper, error) {
	if cfg.CACertFile == "" && !cfg.InsecureSkipVerify {
		return http.DefaultTransport, nil
	}
	settings := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.InsecureSkipVerify {
		settings.InsecureSkipVerify = true
	}
	if cfg.CACertFile != "" {
		pool, err := poolFrom(cfg.CACertFile)
		if err != nil {
			return nil, err
		}
		settings.RootCAs = pool
	}
	cloned, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("the default http transport was replaced; spinoza cannot add your ca certificate to it")
	}
	copied := cloned.Clone()
	copied.TLSClientConfig = settings
	return copied, nil
}

func poolFrom(path string) (*x509.CertPool, error) {
	body, err := filetx.Read(path, maxCACertBytes)
	if err != nil {
		return nil, fmt.Errorf("oidc ca certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(body) {
		return nil, fmt.Errorf("oidc ca certificate %s holds no certificate spinoza could read", path)
	}
	return pool, nil
}

func newProvider(ctx context.Context, cfg OIDCConfig) (*provider, error) {
	client, clientErr := httpClientFor(cfg)
	if clientErr != nil {
		return nil, clientErr
	}
	discoverCtx := oidc.ClientContext(ctx, client)
	found, doc, err := discover(discoverCtx, cfg)
	if err != nil {
		return nil, err
	}
	return &provider{
		cfg: cfg,
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     found.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       askableScopes(cfg.Scopes, doc.ScopesSupported),
		},
		verifier:    found.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		client:      client,
		endSession:  doc.EndSession,
		backchannel: doc.BackchannelLogout,
		sessionIDs:  doc.BackchannelLogoutSession,
	}, nil
}

func discover(ctx context.Context, cfg OIDCConfig) (*oidc.Provider, discovered, error) {
	if cfg.InternalIssuerURL == "" {
		return discoverAt(ctx, cfg.IssuerURL)
	}
	_, doc, err := discoverAt(oidc.InsecureIssuerURLContext(ctx, cfg.IssuerURL), cfg.InternalIssuerURL)
	if err != nil {
		return nil, discovered{}, err
	}
	found, facing := browserFacing(ctx, cfg, doc)
	return found, facing, nil
}

func discoverAt(ctx context.Context, issuer string) (*oidc.Provider, discovered, error) {
	found, err := oidc.NewProvider(ctx, strings.TrimSuffix(issuer, "/"))
	if err != nil {
		return nil, discovered{}, fmt.Errorf("oidc discovery at %s: %w", issuer, err)
	}
	var doc discovered
	claimsErr := found.Claims(&doc)
	if claimsErr != nil {
		return nil, discovered{}, fmt.Errorf("oidc discovery document at %s: %w", issuer, claimsErr)
	}
	return found, doc, nil
}

func browserFacing(ctx context.Context, cfg OIDCConfig, doc discovered) (*oidc.Provider, discovered) {
	internal := strings.TrimSuffix(cfg.InternalIssuerURL, "/")
	public := strings.TrimSuffix(cfg.IssuerURL, "/")
	doc.AuthorizationEndpoint = swapBase(doc.AuthorizationEndpoint, internal, public)
	doc.EndSession = swapBase(doc.EndSession, internal, public)
	built := &oidc.ProviderConfig{
		IssuerURL:   public,
		AuthURL:     doc.AuthorizationEndpoint,
		TokenURL:    doc.TokenEndpoint,
		UserInfoURL: doc.UserInfoEndpoint,
		JWKSURL:     doc.JWKSEndpoint,
		Algorithms:  doc.IDTokenSigningAlgSupported,
	}
	return built.NewProvider(ctx), doc
}

func askableScopes(wanted, supported []string) []string {
	if len(supported) == 0 {
		return wanted
	}
	kept := make([]string, 0, len(wanted))
	dropped := []string{}
	for _, one := range wanted {
		if one == oidc.ScopeOpenID || slices.Contains(supported, one) {
			kept = append(kept, one)
			continue
		}
		dropped = append(dropped, one)
	}
	if len(dropped) > 0 {
		slog.Warn(
			"your identity provider defines none of these scopes, so they were left out and no claim from them will arrive",
			"scopes", strings.Join(dropped, ","),
		)
	}
	return kept
}

func swapBase(endpoint, from, to string) string {
	if endpoint == "" {
		return ""
	}
	if !strings.HasPrefix(endpoint, from) {
		return endpoint
	}
	suffix := strings.TrimPrefix(endpoint, from)
	if suffix == "" {
		return to
	}
	if suffix[0] != '/' && suffix[0] != '?' && suffix[0] != '#' {
		return endpoint
	}
	return to + suffix
}

type flowState struct {
	Nonce    string `json:"n"`
	Return   string `json:"r"`
	State    string `json:"s"`
	Verifier string `json:"v"`
}

func (pr *provider) start(w http.ResponseWriter, r *http.Request, ss *sessions, back string) {
	flow := flowState{
		State:    newSessionID(),
		Verifier: oauth2.GenerateVerifier(),
		Nonce:    newSessionID(),
		Return:   back,
	}
	err := ss.stash(w, flowCookie, flow, flowLifetime)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, err.Error())
		return
	}
	options := []oauth2.AuthCodeOption{
		oauth2.S256ChallengeOption(flow.Verifier),
		oidc.Nonce(flow.Nonce),
	}
	if forcedLogin(r) {
		options = append(options, oauth2.SetAuthURLParam("prompt", "login"))
	}
	http.Redirect(w, r, pr.oauth.AuthCodeURL(flow.State, options...), http.StatusFound)
}

func forcedLogin(r *http.Request) bool {
	return r.URL.Query().Get("prompt") == "login"
}

type claimSet map[string]any

func (pr *provider) finish(ctx context.Context, r *http.Request, ss *sessions) (claimSet, string, error) {
	var flow flowState
	held := ss.unstash(r, &flow)
	if !held {
		return nil, "", errStateMismatch
	}
	if r.URL.Query().Get("state") != flow.State {
		return nil, "", errStateMismatch
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		return nil, "", refusedBy(r)
	}
	exchanged, err := pr.oauth.Exchange(
		oidc.ClientContext(ctx, pr.client),
		code,
		oauth2.VerifierOption(flow.Verifier),
	)
	if err != nil {
		return nil, "", fmt.Errorf("the identity provider refused the login: %w", err)
	}
	raw, ok := exchanged.Extra("id_token").(string)
	if !ok {
		return nil, "", errNoIDToken
	}
	token, verifyErr := pr.verifier.Verify(ctx, raw)
	if verifyErr != nil {
		return nil, "", fmt.Errorf("the id token did not verify: %w", verifyErr)
	}
	if token.Nonce != flow.Nonce {
		return nil, "", errNonceMismatch
	}
	claims := claimSet{}
	claimsErr := token.Claims(&claims)
	if claimsErr != nil {
		return nil, "", fmt.Errorf("the id token could not be read: %w", claimsErr)
	}
	return claims, flow.Return, nil
}

func refusedBy(r *http.Request) error {
	reason := r.URL.Query().Get("error_description")
	if reason == "" {
		reason = r.URL.Query().Get("error")
	}
	if reason == "" {
		return errors.New("the identity provider came back without an authorization code")
	}
	return errors.New("the identity provider refused the login: " + reason)
}

func (pr *provider) logoutURL() string {
	if pr.endSession == "" {
		return ""
	}
	parsed, err := url.Parse(pr.endSession)
	if err != nil {
		return ""
	}
	query := parsed.Query()
	query.Set("client_id", pr.cfg.ClientID)
	if pr.cfg.PostLogoutURL != "" {
		query.Set("post_logout_redirect_uri", pr.cfg.PostLogoutURL)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

type logoutClaims struct {
	Events map[string]any `json:"events"`
	SID    string         `json:"sid"`
	Sub    string         `json:"sub"`
}

//nolint:revive // the event name the openid spec defines, not an address anything is fetched from
const backchannelEvent = "http://schemas.openid.net/event/backchannel-logout"

func (pr *provider) verifyLogoutToken(ctx context.Context, raw string) (logoutClaims, error) {
	token, err := pr.verifier.Verify(ctx, raw)
	if err != nil {
		return logoutClaims{}, fmt.Errorf("the logout token did not verify: %w", err)
	}
	var claims logoutClaims
	claimsErr := token.Claims(&claims)
	if claimsErr != nil {
		return logoutClaims{}, fmt.Errorf("the logout token could not be read: %w", claimsErr)
	}
	_, wanted := claims.Events[backchannelEvent]
	if !wanted {
		return logoutClaims{}, errors.New("the logout token is not a back-channel logout")
	}
	if claims.SID == "" && claims.Sub == "" {
		return logoutClaims{}, errors.New("the logout token names neither a session nor a subject")
	}
	return claims, nil
}
