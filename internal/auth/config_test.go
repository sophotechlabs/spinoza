package auth

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultsFillInEverythingLeftOut(t *testing.T) {
	got := Config{}.withDefaults()

	if got.Mode != ModeNone {
		t.Fatalf("mode = %q, want %q", got.Mode, ModeNone)
	}
	if got.SessionTTL != DefaultSessionTTL {
		t.Fatalf("ttl = %s, want %s", got.SessionTTL, DefaultSessionTTL)
	}
	if got.DefaultRole != RoleViewer {
		t.Fatalf("default role = %q, want %q", got.DefaultRole, RoleViewer)
	}
	if got.Proxy.UserHeader != DefaultUserHeader || got.Proxy.GroupsHeader != DefaultGroupsHeader {
		t.Fatalf("proxy headers = %+v, want the oauth2-proxy ones", got.Proxy)
	}
	if got.OIDC.GroupsClaim != DefaultGroupsClaim {
		t.Fatalf("groups claim = %q, want %q", got.OIDC.GroupsClaim, DefaultGroupsClaim)
	}
	if strings.Join(got.OIDC.UsernameClaims, ",") != DefaultUsernameKeys {
		t.Fatalf("username claims = %v, want %q", got.OIDC.UsernameClaims, DefaultUsernameKeys)
	}
	if strings.Join(got.OIDC.Scopes, " ") != "openid profile email groups" {
		t.Fatalf("scopes = %v, want the four spinoza asks for", got.OIDC.Scopes)
	}
}

func TestDefaultsLeaveWhatWasGivenAlone(t *testing.T) {
	got := Config{
		Mode:        ModeProxy,
		SessionTTL:  time.Minute,
		DefaultRole: RoleAdmin,
		OIDC:        OIDCConfig{Scopes: []string{"openid"}},
	}.withDefaults()

	if got.SessionTTL != time.Minute || got.DefaultRole != RoleAdmin {
		t.Fatalf("config = %+v, want the values that were set", got)
	}
	if len(got.OIDC.Scopes) != 1 {
		t.Fatalf("scopes = %v, want only the one that was asked for", got.OIDC.Scopes)
	}
}

func TestValidateRefusesWhatCannotWork(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "a mode nobody implements",
			cfg:  Config{Mode: "ldap", DefaultRole: RoleViewer},
			want: `auth mode "ldap" is not one of`,
		},
		{
			name: "a role nobody defines",
			cfg:  Config{Mode: ModeNone, DefaultRole: "owner"},
			want: `default role "owner" is not one of`,
		},
		{
			name: "oidc with no issuer",
			cfg:  Config{Mode: ModeOIDC, DefaultRole: RoleViewer},
			want: "oidc needs an issuer url",
		},
		{
			name: "oidc with no client",
			cfg:  Config{Mode: ModeOIDC, DefaultRole: RoleViewer, OIDC: OIDCConfig{IssuerURL: "https://idp"}},
			want: "oidc needs a client id",
		},
		{
			name: "oidc with no redirect",
			cfg: Config{Mode: ModeOIDC, DefaultRole: RoleViewer, OIDC: OIDCConfig{
				IssuerURL: "https://idp", ClientID: "spinoza",
			}},
			want: "oidc needs a redirect url",
		},
		{
			name: "a redirect that is only a path",
			cfg: Config{Mode: ModeOIDC, DefaultRole: RoleViewer, OIDC: OIDCConfig{
				IssuerURL: "https://idp", ClientID: "spinoza", RedirectURL: "/auth/callback",
			}},
			want: "must be absolute",
		},
		{
			name: "a redirect that is not a url",
			cfg: Config{Mode: ModeOIDC, DefaultRole: RoleViewer, OIDC: OIDCConfig{
				IssuerURL: "https://idp", ClientID: "spinoza", RedirectURL: "://nope",
			}},
			want: "oidc redirect url",
		},
		{
			name: "both a ca and no verification",
			cfg: Config{Mode: ModeOIDC, DefaultRole: RoleViewer, OIDC: OIDCConfig{
				IssuerURL:          "https://idp",
				ClientID:           "spinoza",
				RedirectURL:        "https://spinoza.example.com/auth/callback",
				CACertFile:         "/etc/ca.crt",
				InsecureSkipVerify: true,
			}},
			want: "either a ca certificate or skipped verification",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if err == nil {
				t.Fatal("the config was accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestValidateAcceptsAWorkableOIDCConfig(t *testing.T) {
	cfg := Config{
		Mode:        ModeOIDC,
		DefaultRole: RoleViewer,
		OIDC: OIDCConfig{
			IssuerURL:   "https://keycloak.example.com/realms/main",
			ClientID:    "spinoza",
			RedirectURL: "https://spinoza.example.com/auth/callback",
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("a workable config was refused: %v", err)
	}
}

func TestParseListDropsBlanksAndSpaces(t *testing.T) {
	got := ParseList(" platform , , sre ")

	if len(got) != 2 || got[0] != "platform" || got[1] != "sre" {
		t.Fatalf("list = %v, want [platform sre]", got)
	}
	if len(ParseList("")) != 0 {
		t.Fatal("an empty string produced entries")
	}
}

func TestASignInMayNotOutliveWhatItMayBeRenewedFor(t *testing.T) {
	cfg := Config{Mode: ModeNone, DefaultRole: RoleViewer, SessionTTL: time.Hour, SessionMaxAge: time.Minute}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("a cap shorter than the ttl was accepted")
	}
	if !strings.Contains(err.Error(), "may not last longer") {
		t.Fatalf("error = %q, want it to say why", err.Error())
	}
}

func TestTheSessionCapHasADefault(t *testing.T) {
	if got := (Config{}).withDefaults().SessionMaxAge; got != DefaultSessionMax {
		t.Fatalf("cap = %s, want %s", got, DefaultSessionMax)
	}
}

func TestValidateRejectsAWeakExplicitSessionSecret(t *testing.T) {
	cfg := Config{
		Mode:          ModeNone,
		DefaultRole:   RoleViewer,
		SessionSecret: []byte(strings.Repeat("x", minimumSecretBytes-1)),
	}

	err := cfg.Validate()

	if err == nil {
		t.Fatal("a short session secret was accepted")
	}
	if !strings.Contains(err.Error(), "at least 32 bytes") {
		t.Fatalf("error = %q, want the minimum length", err.Error())
	}
}

func TestValidateAcceptsGeneratedAndStrongSessionSecrets(t *testing.T) {
	for _, secret := range [][]byte{nil, []byte(strings.Repeat("x", minimumSecretBytes))} {
		cfg := Config{Mode: ModeNone, DefaultRole: RoleViewer, SessionSecret: secret}

		if err := cfg.Validate(); err != nil {
			t.Fatalf("a %d-byte session secret was refused: %v", len(secret), err)
		}
	}
}
