package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

func fileHolding(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func servedFlags(t *testing.T, extra ...string) serving {
	t.Helper()
	args := append([]string{"--cluster-mode", "--public-url", "https://spinoza.example.com"}, extra...)
	opts, err := parseFlags(args)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return opts.serve
}

func TestWithoutClusterModeNothingAboutServingIsSetUp(t *testing.T) {
	opts, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if opts.serve.on {
		t.Fatal("a plain run was set up to serve a cluster")
	}
	if opts.cluster.Impersonate {
		t.Fatal("a plain run would impersonate somebody")
	}
	if opts.addr != "127.0.0.1:34115" {
		t.Fatalf("addr = %q, want the loopback default", opts.addr)
	}
}

func TestServingAClusterNeedsToKnowWhereBrowsersReachIt(t *testing.T) {
	_, err := parseFlags([]string{"--cluster-mode"})

	if err == nil {
		t.Fatal("cluster mode started without a public url")
	}
	if !strings.Contains(err.Error(), "--public-url") {
		t.Fatalf("error = %q, want it to name the flag", err.Error())
	}
}

func TestAPublicUrlHasToBeOne(t *testing.T) {
	cases := map[string]string{
		"spinoza.example.com": "http:// or https://",
		"https://":            "names no host",
		"://nope":             "public url",
	}
	for given, want := range cases {
		t.Run(given, func(t *testing.T) {
			_, err := parseFlags([]string{"--cluster-mode", "--public-url", given})
			if err == nil {
				t.Fatal("an unusable public url was accepted")
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %q, want it to mention %q", err.Error(), want)
			}
		})
	}
}

func TestServingMovesTheListenAddressOffLoopback(t *testing.T) {
	if got := servedListen(t); got != clusterAddr {
		t.Fatalf("addr = %q, want %q so the service can reach the pod", got, clusterAddr)
	}
}

func servedListen(t *testing.T, extra ...string) string {
	t.Helper()
	args := append([]string{"--cluster-mode", "--public-url", "https://spinoza.example.com"}, extra...)
	opts, err := parseFlags(args)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return opts.addr
}

func TestAnAddressYouNamedYourselfIsKept(t *testing.T) {
	if got := servedListen(t, "--addr", "127.0.0.1:9000"); got != "127.0.0.1:9000" {
		t.Fatalf("addr = %q, want the one that was asked for", got)
	}
}

func TestAnAddressFromTheEnvironmentIsKept(t *testing.T) {
	t.Setenv("SPINOZA_ADDR", "0.0.0.0:9999")

	if got := servedListen(t); got != "0.0.0.0:9999" {
		t.Fatalf("addr = %q, want the one in the environment", got)
	}
}

func TestTheCallbackAndTheLandingPageComeFromThePublicUrl(t *testing.T) {
	got := servedFlags(t)

	if got.auth.OIDC.RedirectURL != "https://spinoza.example.com/auth/callback" {
		t.Fatalf("redirect = %q", got.auth.OIDC.RedirectURL)
	}
	if got.auth.OIDC.PostLogoutURL != "https://spinoza.example.com/" {
		t.Fatalf("post logout = %q", got.auth.OIDC.PostLogoutURL)
	}
}

func TestARedirectYouRegisteredYourselfIsKept(t *testing.T) {
	got := servedFlags(
		t,
		"--auth-oidc-redirect-url", "https://tools.example.com/spinoza/auth/callback",
		"--auth-oidc-post-logout-url", "https://tools.example.com/spinoza/",
	)

	if got.auth.OIDC.RedirectURL != "https://tools.example.com/spinoza/auth/callback" {
		t.Fatalf("redirect = %q", got.auth.OIDC.RedirectURL)
	}
	if got.auth.OIDC.PostLogoutURL != "https://tools.example.com/spinoza/" {
		t.Fatalf("post logout = %q", got.auth.OIDC.PostLogoutURL)
	}
}

func TestServingActsAsWhoeverIsSignedInUnlessYouSayOtherwise(t *testing.T) {
	opts, err := parseFlags([]string{"--cluster-mode", "--public-url", "https://spinoza.example.com"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !opts.cluster.Impersonate {
		t.Fatal("cluster mode would act as spinoza's own service account by default")
	}

	off, offErr := parseFlags([]string{
		"--cluster-mode", "--public-url", "https://spinoza.example.com", "--impersonate=false",
	})
	if offErr != nil {
		t.Fatalf("parse: %v", offErr)
	}
	if off.cluster.Impersonate {
		t.Fatal("impersonation stayed on after it was turned off")
	}
}

func TestEveryAuthFlagReachesTheConfig(t *testing.T) {
	got := servedFlags(
		t,
		"--auth-mode", "oidc",
		"--session-ttl", "30m",
		"--auth-default-role", "editor",
		"--auth-admin-groups", "platform-admins",
		"--auth-editor-groups", "platform, sre",
		"--auth-viewer-groups", "everyone",
		"--auth-user-header", "X-User",
		"--auth-groups-header", "X-Groups",
		"--auth-proxy-logout-url", "https://proxy/sign_out",
		"--auth-oidc-issuer", "https://keycloak.example.com/realms/main",
		"--auth-oidc-internal-issuer", "http://keycloak.keycloak.svc/realms/main",
		"--auth-oidc-client-id", "spinoza",
		"--auth-oidc-client-secret", "shh",
		"--auth-oidc-scopes", "openid,profile",
		"--auth-oidc-groups-claim", "roles",
		"--auth-oidc-username-claims", "email,sub",
		"--auth-oidc-username-prefix", "oidc:",
		"--auth-oidc-groups-prefix", "oidc:",
		"--auth-oidc-ca-cert", "/etc/spinoza/ca.crt",
		"--auth-oidc-backchannel-logout",
	)

	if got.auth.Mode != auth.ModeOIDC {
		t.Fatalf("mode = %q", got.auth.Mode)
	}
	if got.auth.SessionTTL != 30*time.Minute {
		t.Fatalf("ttl = %s", got.auth.SessionTTL)
	}
	if got.auth.DefaultRole != auth.RoleEditor {
		t.Fatalf("default role = %q", got.auth.DefaultRole)
	}
	if strings.Join(got.auth.EditorGroups, ",") != "platform,sre" {
		t.Fatalf("editor groups = %v", got.auth.EditorGroups)
	}
	if got.auth.AdminGroups[0] != "platform-admins" || got.auth.ViewerGroups[0] != "everyone" {
		t.Fatalf("groups = %+v", got.auth)
	}
	if got.auth.Proxy.UserHeader != "X-User" || got.auth.Proxy.GroupsHeader != "X-Groups" {
		t.Fatalf("proxy = %+v", got.auth.Proxy)
	}
	if got.auth.Proxy.LogoutURL != "https://proxy/sign_out" {
		t.Fatalf("proxy logout = %q", got.auth.Proxy.LogoutURL)
	}
	oidc := got.auth.OIDC
	if oidc.IssuerURL != "https://keycloak.example.com/realms/main" {
		t.Fatalf("issuer = %q", oidc.IssuerURL)
	}
	if oidc.InternalIssuerURL != "http://keycloak.keycloak.svc/realms/main" {
		t.Fatalf("internal issuer = %q", oidc.InternalIssuerURL)
	}
	if oidc.ClientID != "spinoza" || oidc.ClientSecret != "shh" {
		t.Fatalf("client = %q/%q", oidc.ClientID, oidc.ClientSecret)
	}
	if strings.Join(oidc.Scopes, " ") != "openid profile" {
		t.Fatalf("scopes = %v", oidc.Scopes)
	}
	if oidc.GroupsClaim != "roles" || strings.Join(oidc.UsernameClaims, ",") != "email,sub" {
		t.Fatalf("claims = %q/%v", oidc.GroupsClaim, oidc.UsernameClaims)
	}
	if oidc.UsernamePrefix != "oidc:" || oidc.GroupsPrefix != "oidc:" {
		t.Fatalf("prefixes = %q/%q", oidc.UsernamePrefix, oidc.GroupsPrefix)
	}
	if oidc.CACertFile != "/etc/spinoza/ca.crt" || !oidc.BackchannelLogout {
		t.Fatalf("oidc = %+v", oidc)
	}
}

func TestSecretsComeFromFilesWithoutTheirTrailingNewline(t *testing.T) {
	got := servedFlags(
		t,
		"--session-secret-file", fileHolding(t, "signing-key\n"),
		"--auth-oidc-client-secret-file", fileHolding(t, "client-secret\n"),
		"--auth-oidc-client-secret", "ignored",
	)

	if string(got.auth.SessionSecret) != "signing-key" {
		t.Fatalf("session secret = %q", string(got.auth.SessionSecret))
	}
	if got.auth.OIDC.ClientSecret != "client-secret" {
		t.Fatalf("client secret = %q, want the file to win over the flag", got.auth.OIDC.ClientSecret)
	}
}

func TestASecretFileThatIsNotThereStopsSpinozaStarting(t *testing.T) {
	for _, flag := range []string{"--session-secret-file", "--auth-oidc-client-secret-file"} {
		t.Run(flag, func(t *testing.T) {
			_, err := parseFlags([]string{
				"--cluster-mode", "--public-url", "https://spinoza.example.com", flag, "/no/such/file",
			})
			if err == nil {
				t.Fatal("a missing secret file was accepted")
			}
			if !strings.Contains(err.Error(), "/no/such/file") {
				t.Fatalf("error = %q, want it to name the file", err.Error())
			}
		})
	}
}

func TestTheSessionSecretCanComeFromTheEnvironment(t *testing.T) {
	t.Setenv(secretEnv, "from-the-environment")

	if string(servedFlags(t).auth.SessionSecret) != "from-the-environment" {
		t.Fatal("the session secret in the environment was not read")
	}
}

func TestAnEmptyEnvironmentValueLeavesTheDefaultAlone(t *testing.T) {
	t.Setenv("SPINOZA_IMPERSONATE", "")

	if !envUnless("SPINOZA_IMPERSONATE") {
		t.Fatal("an empty environment variable turned a default off")
	}
	t.Setenv("SPINOZA_IMPERSONATE", "false")
	if envUnless("SPINOZA_IMPERSONATE") {
		t.Fatal("the environment did not turn the default off")
	}
	if !envUnless("SPINOZA_NOT_SET_ANYWHERE") {
		t.Fatal("a variable nobody set changed the default")
	}
}

func TestTheVersionAndTheLicenceStillPrintInsideTheImage(t *testing.T) {
	t.Setenv("SPINOZA_CLUSTER_MODE", "true")

	for _, flag := range []string{"--version", "--license"} {
		t.Run(flag, func(t *testing.T) {
			opts, err := parseFlags([]string{flag})
			if err != nil {
				t.Fatalf("%s asked for the public url before printing: %v", flag, err)
			}
			if !printedNotice(io.Discard, opts) {
				t.Fatalf("%s printed nothing", flag)
			}
		})
	}
}
