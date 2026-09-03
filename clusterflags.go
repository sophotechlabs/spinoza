package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/filetx"
)

const (
	clusterAddr  = "0.0.0.0:8080"
	callbackPath = "/auth/callback"
	//nolint:gosec // the name of an environment variable, not a secret
	secretEnv = "SPINOZA_SESSION_SECRET"
	//nolint:gosec // the name of an environment variable, not a secret
	clientSecretEnv = "SPINOZA_AUTH_OIDC_CLIENT_SECRET"
	proxyAuthEnv    = "SPINOZA_AUTH_PROXY_SECRET"
	maxSecretBytes  = 1 << 20
)

type serving struct {
	on          bool
	publicURL   string
	unsafeHTTP  bool
	impersonate bool
	auth        auth.Config
}

type clusterFlags struct {
	on            *bool
	publicURL     *string
	unsafeHTTP    *bool
	impersonate   *bool
	mode          *string
	allowAnon     *bool
	secretFile    *string
	ttl           *time.Duration
	maxAge        *time.Duration
	defaultRole   *string
	adminGroups   *string
	editorGroups  *string
	viewerGroups  *string
	userHeader    *string
	groupsHeader  *string
	proxyHeader   *string
	proxyFile     *string
	proxyLogout   *string
	proxyWSMaxAge *time.Duration
	issuer        *string
	inner         *string
	clientID      *string
	clientSecret  *string
	secretPath    *string
	redirect      *string
	scopes        *string
	groupsClaim   *string
	usernameClaim *string
	userPrefix    *string
	groupPrefix   *string
	postLogout    *string
	caCert        *string
	skipVerify    *bool
	issuerHTTP    *bool
	backchannel   *bool
}

func registerCluster(flags *flag.FlagSet) *clusterFlags {
	return &clusterFlags{
		on:            flags.Bool("cluster-mode", envBool("SPINOZA_CLUSTER_MODE"), "serve a cluster to a team instead of running as your own local window"),
		publicURL:     flags.String("public-url", envOr("SPINOZA_PUBLIC_URL", ""), "the http(s) origin browsers reach spinoza at, such as https://spinoza.example.com"),
		unsafeHTTP:    flags.Bool("unsafe-allow-http", envBool("SPINOZA_UNSAFE_ALLOW_HTTP"), "allow plaintext non-loopback browser sessions; exposes credentials and cluster data to the network"),
		impersonate:   flags.Bool("impersonate", envUnless("SPINOZA_IMPERSONATE"), "act on the cluster as the signed-in user, so kubernetes rbac decides what they may do"),
		mode:          flags.String("auth-mode", envOr("SPINOZA_AUTH_MODE", auth.ModeNone), "how people sign in: none, proxy or oidc"),
		allowAnon:     flags.Bool("allow-anonymous-admin", envBool("SPINOZA_ALLOW_ANONYMOUS_ADMIN"), "allow unauthenticated cluster-mode requests to act as administrators"),
		secretFile:    flags.String("session-secret-file", envOr("SPINOZA_SESSION_SECRET_FILE", ""), "file holding the key that signs session cookies; without one every sign-in ends when spinoza restarts"),
		ttl:           flags.Duration("session-ttl", envDuration("SPINOZA_SESSION_TTL", auth.DefaultSessionTTL), "how long a sign-in lasts before it is renewed or ends"),
		maxAge:        flags.Duration("session-max-age", envDuration("SPINOZA_SESSION_MAX_AGE", auth.DefaultSessionMax), "how long a sign-in may be renewed for before the provider has to decide again"),
		defaultRole:   flags.String("auth-default-role", envOr("SPINOZA_AUTH_DEFAULT_ROLE", auth.RoleViewer), "role for anyone whose groups match none of the lists: viewer, editor or admin"),
		adminGroups:   flags.String("auth-admin-groups", envOr("SPINOZA_AUTH_ADMIN_GROUPS", ""), "comma separated groups whose members are admins here"),
		editorGroups:  flags.String("auth-editor-groups", envOr("SPINOZA_AUTH_EDITOR_GROUPS", ""), "comma separated groups whose members may change objects"),
		viewerGroups:  flags.String("auth-viewer-groups", envOr("SPINOZA_AUTH_VIEWER_GROUPS", ""), "comma separated groups whose members may only look"),
		userHeader:    flags.String("auth-user-header", envOr("SPINOZA_AUTH_USER_HEADER", auth.DefaultUserHeader), "header your auth proxy puts the username in"),
		groupsHeader:  flags.String("auth-groups-header", envOr("SPINOZA_AUTH_GROUPS_HEADER", auth.DefaultGroupsHeader), "header your auth proxy puts the groups in"),
		proxyHeader:   flags.String("auth-proxy-secret-header", envOr("SPINOZA_AUTH_PROXY_SECRET_HEADER", auth.DefaultProxyAuthHeader), "header your auth proxy puts its shared secret in"),
		proxyFile:     flags.String("auth-proxy-secret-file", envOr("SPINOZA_AUTH_PROXY_SECRET_FILE", ""), "file holding the secret that authenticates your proxy"),
		proxyLogout:   flags.String("auth-proxy-logout-url", envOr("SPINOZA_AUTH_PROXY_LOGOUT_URL", ""), "where signing out sends the browser when a proxy holds the session"),
		proxyWSMaxAge: flags.Duration("auth-proxy-websocket-max-age", envDuration("SPINOZA_AUTH_PROXY_WEBSOCKET_MAX_AGE", auth.DefaultProxyWSMaxAge), "maximum lifetime of a proxy-authenticated websocket before fresh proxy headers are required"),
		issuer:        flags.String("auth-oidc-issuer", envOr("SPINOZA_AUTH_OIDC_ISSUER", ""), "your identity provider, such as https://keycloak.example.com/realms/main"),
		inner:         flags.String("auth-oidc-internal-issuer", envOr("SPINOZA_AUTH_OIDC_INTERNAL_ISSUER", ""), "the same provider on the address this pod can reach, when it differs from the browser's"),
		clientID:      flags.String("auth-oidc-client-id", envOr("SPINOZA_AUTH_OIDC_CLIENT_ID", ""), "the client spinoza is registered as"),
		clientSecret:  flags.String("auth-oidc-client-secret", envOr(clientSecretEnv, ""), "the client secret; prefer the file flag or the environment variable"),
		secretPath:    flags.String("auth-oidc-client-secret-file", envOr("SPINOZA_AUTH_OIDC_CLIENT_SECRET_FILE", ""), "file holding the client secret"),
		redirect:      flags.String("auth-oidc-redirect-url", envOr("SPINOZA_AUTH_OIDC_REDIRECT_URL", ""), "where the provider sends the browser back; the public url plus /auth/callback when empty"),
		scopes:        flags.String("auth-oidc-scopes", envOr("SPINOZA_AUTH_OIDC_SCOPES", strings.Join(auth.DefaultScopes, ",")), "comma separated scopes to ask for"),
		groupsClaim:   flags.String("auth-oidc-groups-claim", envOr("SPINOZA_AUTH_OIDC_GROUPS_CLAIM", auth.DefaultGroupsClaim), "claim in the id token that carries group membership"),
		usernameClaim: flags.String("auth-oidc-username-claims", envOr("SPINOZA_AUTH_OIDC_USERNAME_CLAIMS", auth.DefaultUsernameKeys), "comma separated claims to read a username from, first one present wins"),
		userPrefix:    flags.String("auth-oidc-username-prefix", envOr("SPINOZA_AUTH_OIDC_USERNAME_PREFIX", ""), "prefix put in front of the username, to match what your apiserver binds"),
		groupPrefix:   flags.String("auth-oidc-groups-prefix", envOr("SPINOZA_AUTH_OIDC_GROUPS_PREFIX", ""), "prefix put in front of every group, to match what your apiserver binds"),
		postLogout:    flags.String("auth-oidc-post-logout-url", envOr("SPINOZA_AUTH_OIDC_POST_LOGOUT_URL", ""), "where the provider sends the browser after signing out; register it there first"),
		caCert:        flags.String("auth-oidc-ca-cert", envOr("SPINOZA_AUTH_OIDC_CA_CERT", ""), "certificate authority to trust when talking to the provider"),
		skipVerify:    flags.Bool("auth-oidc-insecure-skip-verify", envBool("SPINOZA_AUTH_OIDC_INSECURE_SKIP_VERIFY"), "do not verify the provider's certificate; for a lab, never for real use"),
		issuerHTTP:    flags.Bool("auth-oidc-unsafe-allow-http", envBool("SPINOZA_AUTH_OIDC_UNSAFE_ALLOW_HTTP"), "allow plaintext non-loopback oidc endpoints; permits network authentication compromise"),
		backchannel:   flags.Bool("auth-oidc-backchannel-logout", envBool("SPINOZA_AUTH_OIDC_BACKCHANNEL_LOGOUT"), "accept back-channel logout, so disabling somebody at the provider ends their session here"),
	}
}

func (cf *clusterFlags) settings() (serving, error) {
	out := serving{
		on:          *cf.on,
		publicURL:   *cf.publicURL,
		unsafeHTTP:  *cf.unsafeHTTP,
		impersonate: *cf.impersonate,
	}
	if !out.on {
		return out, nil
	}
	secret, secretErr := readSecret(*cf.secretFile, secretEnv)
	if secretErr != nil {
		return serving{}, secretErr
	}
	clientSecret, clientErr := readSecret(*cf.secretPath, "")
	if clientErr != nil {
		return serving{}, clientErr
	}
	if len(clientSecret) == 0 {
		clientSecret = []byte(*cf.clientSecret)
	}
	proxySecret, proxySecretErr := readSecret(*cf.proxyFile, proxyAuthEnv)
	if proxySecretErr != nil {
		return serving{}, proxySecretErr
	}
	out.auth = auth.Config{
		Mode:           *cf.mode,
		PublicURL:      out.publicURL,
		AllowAnonymous: *cf.allowAnon,
		SessionSecret:  secret,
		SessionTTL:     *cf.ttl,
		SessionMaxAge:  *cf.maxAge,
		DefaultRole:    *cf.defaultRole,
		AdminGroups:    auth.ParseList(*cf.adminGroups),
		EditorGroups:   auth.ParseList(*cf.editorGroups),
		ViewerGroups:   auth.ParseList(*cf.viewerGroups),
		Proxy: auth.ProxyConfig{
			UserHeader:      *cf.userHeader,
			GroupsHeader:    *cf.groupsHeader,
			SecretHeader:    *cf.proxyHeader,
			SharedSecret:    proxySecret,
			LogoutURL:       *cf.proxyLogout,
			WebSocketMaxAge: *cf.proxyWSMaxAge,
		},
		OIDC: auth.OIDCConfig{
			IssuerURL:          *cf.issuer,
			InternalIssuerURL:  *cf.inner,
			ClientID:           *cf.clientID,
			ClientSecret:       string(clientSecret),
			RedirectURL:        underPublic(*cf.redirect, out.publicURL, callbackPath),
			Scopes:             auth.ParseList(*cf.scopes),
			GroupsClaim:        *cf.groupsClaim,
			UsernameClaims:     auth.ParseList(*cf.usernameClaim),
			UsernamePrefix:     *cf.userPrefix,
			GroupsPrefix:       *cf.groupPrefix,
			PostLogoutURL:      underPublic(*cf.postLogout, out.publicURL, "/"),
			CACertFile:         *cf.caCert,
			InsecureSkipVerify: *cf.skipVerify,
			UnsafeAllowHTTP:    *cf.issuerHTTP,
			BackchannelLogout:  *cf.backchannel,
		},
	}
	return out, nil
}

func (sv serving) check() error {
	if !sv.on {
		return nil
	}
	parsed, err := sv.parsePublicURL()
	if err != nil {
		return err
	}
	if err := sv.checkPublicTransport(parsed); err != nil {
		return err
	}
	if err := sv.checkPublicShape(parsed); err != nil {
		return err
	}
	if sv.unsafeHTTP {
		slog.Warn("plaintext public http is enabled; network interception can steal sessions and cluster data")
	}
	return nil
}

func (sv serving) parsePublicURL() (*url.URL, error) {
	if sv.publicURL == "" {
		return nil, errors.New("cluster mode needs --public-url, the address browsers reach spinoza at")
	}
	parsed, err := url.Parse(sv.publicURL)
	if err != nil {
		return nil, fmt.Errorf("public url %q: %w", sv.publicURL, err)
	}
	return parsed, nil
}

func (sv serving) checkPublicTransport(parsed *url.URL) error {
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("public url %q needs to start with http:// or https://", sv.publicURL)
	}
	if parsed.Scheme == "http" && !loopbackEndpoint(parsed.Hostname()) && !sv.unsafeHTTP {
		return fmt.Errorf("public url %q must use https; plaintext http requires --unsafe-allow-http", sv.publicURL)
	}
	return nil
}

func (sv serving) checkPublicShape(parsed *url.URL) error {
	if parsed.Host == "" {
		return fmt.Errorf("public url %q names no host", sv.publicURL)
	}
	if parsed.User != nil {
		return fmt.Errorf("public url %q must not include credentials", sv.publicURL)
	}
	if escaped := parsed.EscapedPath(); escaped != "" && escaped != "/" {
		return fmt.Errorf("public url %q must not include a path", sv.publicURL)
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return fmt.Errorf("public url %q must not include a query", sv.publicURL)
	}
	if strings.Contains(sv.publicURL, "#") {
		return fmt.Errorf("public url %q must not include a fragment", sv.publicURL)
	}
	return nil
}

func underPublic(given, public, path string) string {
	if given != "" {
		return given
	}
	if public == "" {
		return ""
	}
	return strings.TrimSuffix(public, "/") + path
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

func readSecret(path, fallbackEnv string) ([]byte, error) {
	if path == "" {
		if fallbackEnv == "" {
			return nil, nil
		}
		return []byte(os.Getenv(fallbackEnv)), nil
	}
	body, err := filetx.Read(path, maxSecretBytes)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return []byte(strings.TrimSpace(string(body))), nil
}

func envUnless(name string) bool {
	value, present := os.LookupEnv(name)
	if !present {
		return true
	}
	if value == "" {
		return true
	}
	return envBool(name)
}
