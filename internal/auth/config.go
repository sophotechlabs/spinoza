package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	ModeNone  = "none"
	ModeProxy = "proxy"
	ModeOIDC  = "oidc"
)

const (
	DefaultUserHeader      = "X-Forwarded-User"
	DefaultGroupsHeader    = "X-Forwarded-Groups"
	DefaultProxyAuthHeader = "X-Spinoza-Proxy-Secret"
	DefaultGroupsClaim     = "groups"
	DefaultUsernameKeys    = "preferred_username,email,sub"
	DefaultSessionTTL      = 8 * time.Hour
	DefaultSessionMax      = 24 * time.Hour
	DefaultProxyWSMaxAge   = 5 * time.Minute
	maximumProxyWSMaxAge   = 15 * time.Minute
	minimumSecretBytes     = 32
)

var DefaultScopes = []string{"openid", "profile", "email", "groups"}

var modes = []string{ModeNone, ModeProxy, ModeOIDC}

type ProxyConfig struct {
	UserHeader      string
	GroupsHeader    string
	SecretHeader    string
	SharedSecret    []byte
	LogoutURL       string
	WebSocketMaxAge time.Duration
}

type OIDCConfig struct {
	IssuerURL          string
	InternalIssuerURL  string
	ClientID           string
	ClientSecret       string
	RedirectURL        string
	Scopes             []string
	GroupsClaim        string
	UsernameClaims     []string
	UsernamePrefix     string
	GroupsPrefix       string
	PostLogoutURL      string
	CACertFile         string
	InsecureSkipVerify bool
	UnsafeAllowHTTP    bool
	BackchannelLogout  bool
}

type Config struct {
	Mode           string
	PublicURL      string
	AllowAnonymous bool
	SessionSecret  []byte
	SessionTTL     time.Duration
	SessionMaxAge  time.Duration
	DefaultRole    string
	AdminGroups    []string
	EditorGroups   []string
	ViewerGroups   []string
	Proxy          ProxyConfig
	OIDC           OIDCConfig
}

func (cfg Config) withDefaults() Config {
	if cfg.Mode == "" {
		cfg.Mode = ModeNone
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = DefaultSessionTTL
	}
	if cfg.SessionMaxAge <= 0 {
		cfg.SessionMaxAge = DefaultSessionMax
	}
	if cfg.DefaultRole == "" {
		cfg.DefaultRole = RoleViewer
	}
	if cfg.Proxy.UserHeader == "" {
		cfg.Proxy.UserHeader = DefaultUserHeader
	}
	if cfg.Proxy.GroupsHeader == "" {
		cfg.Proxy.GroupsHeader = DefaultGroupsHeader
	}
	if cfg.Proxy.SecretHeader == "" {
		cfg.Proxy.SecretHeader = DefaultProxyAuthHeader
	}
	if cfg.Proxy.WebSocketMaxAge == 0 {
		cfg.Proxy.WebSocketMaxAge = DefaultProxyWSMaxAge
	}
	if cfg.OIDC.GroupsClaim == "" {
		cfg.OIDC.GroupsClaim = DefaultGroupsClaim
	}
	if len(cfg.OIDC.UsernameClaims) == 0 {
		cfg.OIDC.UsernameClaims = strings.Split(DefaultUsernameKeys, ",")
	}
	if len(cfg.OIDC.Scopes) == 0 {
		cfg.OIDC.Scopes = slices.Clone(DefaultScopes)
	}
	return cfg
}

func (cfg Config) Validate() error {
	if !slices.Contains(modes, cfg.Mode) {
		return fmt.Errorf("auth mode %q is not one of %s", cfg.Mode, strings.Join(modes, ", "))
	}
	if err := cfg.validateSession(); err != nil {
		return err
	}
	if cfg.Mode == ModeNone && cfg.PublicURL != "" && !cfg.AllowAnonymous {
		return errors.New("cluster mode without authentication needs explicit anonymous admin access")
	}
	if cfg.Mode == ModeProxy {
		if err := cfg.Proxy.validate(); err != nil {
			return err
		}
	}
	if !KnownRole(cfg.DefaultRole) {
		return fmt.Errorf("default role %q is not one of %s", cfg.DefaultRole, strings.Join(rolesWeakestFirst, ", "))
	}
	if cfg.Mode != ModeOIDC {
		return nil
	}
	return cfg.OIDC.validate()
}

func (cfg Config) validateSession() error {
	if cfg.SessionMaxAge < cfg.SessionTTL {
		return fmt.Errorf("a sign-in may not last longer than it may be renewed for: ttl %s, cap %s", cfg.SessionTTL, cfg.SessionMaxAge)
	}
	if len(cfg.SessionSecret) > 0 && len(cfg.SessionSecret) < minimumSecretBytes {
		return fmt.Errorf("the session secret must be at least %d bytes", minimumSecretBytes)
	}
	return nil
}

func (pc ProxyConfig) validate() error {
	if len(pc.SharedSecret) < minimumSecretBytes {
		return fmt.Errorf("proxy authentication needs a shared secret of at least %d bytes", minimumSecretBytes)
	}
	if pc.WebSocketMaxAge < 0 {
		return errors.New("proxy websocket max age must be positive")
	}
	if pc.WebSocketMaxAge > maximumProxyWSMaxAge {
		return fmt.Errorf("proxy websocket max age may not exceed %s", maximumProxyWSMaxAge)
	}
	return nil
}

func (pc ProxyConfig) authenticates(raw string) bool {
	return subtle.ConstantTimeCompare([]byte(raw), pc.SharedSecret) == 1
}

func (oc OIDCConfig) validate() error {
	if oc.IssuerURL == "" {
		return errors.New("oidc needs an issuer url; point it at your keycloak realm")
	}
	if oc.ClientID == "" {
		return errors.New("oidc needs a client id")
	}
	if err := oc.validateIssuerEndpoints(); err != nil {
		return err
	}
	return oc.validateRedirect()
}

func (oc OIDCConfig) validateIssuerEndpoints() error {
	if err := secureEndpoint("oidc issuer url", oc.IssuerURL, oc.UnsafeAllowHTTP); err != nil {
		return err
	}
	if oc.InternalIssuerURL == "" {
		return nil
	}
	return secureEndpoint("oidc internal issuer url", oc.InternalIssuerURL, oc.UnsafeAllowHTTP)
}

func (oc OIDCConfig) validateRedirect() error {
	if oc.RedirectURL == "" {
		return errors.New("oidc needs a redirect url, the address your identity provider sends the browser back to")
	}
	parsed, err := url.Parse(oc.RedirectURL)
	if err != nil {
		return fmt.Errorf("oidc redirect url %q: %w", oc.RedirectURL, err)
	}
	if !parsed.IsAbs() {
		return fmt.Errorf("oidc redirect url %q must be absolute, scheme and host included", oc.RedirectURL)
	}
	if parsed.Host == "" {
		return fmt.Errorf("oidc redirect url %q must be absolute, scheme and host included", oc.RedirectURL)
	}
	if !webScheme(parsed.Scheme) {
		return fmt.Errorf("oidc redirect url %q must use http or https", oc.RedirectURL)
	}
	if parsed.User != nil {
		return fmt.Errorf("oidc redirect url %q must not include credentials", oc.RedirectURL)
	}
	if strings.Contains(oc.RedirectURL, "#") {
		return fmt.Errorf("oidc redirect url %q must not include a fragment", oc.RedirectURL)
	}
	if oc.InsecureSkipVerify && oc.CACertFile != "" {
		return errors.New("oidc takes either a ca certificate or skipped verification, not both")
	}
	return nil
}

func webScheme(scheme string) bool {
	return scheme == "http" || scheme == "https"
}

func secureEndpoint(name, raw string, unsafeHTTP bool) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s %q: %w", name, raw, err)
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return fmt.Errorf("%s %q must be absolute, scheme and host included", name, raw)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s %q must not include credentials", name, raw)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme != "http" {
		return fmt.Errorf("%s %q must use https", name, raw)
	}
	if loopbackEndpoint(parsed.Hostname()) {
		return nil
	}
	if unsafeHTTP {
		return nil
	}
	return fmt.Errorf("%s %q must use https; plaintext http requires the explicit unsafe opt-in", name, raw)
}

func loopbackEndpoint(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func ParseList(raw string) []string {
	out := []string{}
	for part := range strings.SplitSeq(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
