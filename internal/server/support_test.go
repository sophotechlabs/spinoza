package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	settingsstore "github.com/sophotechlabs/spinoza/internal/settings"
)

var urlWithLogin = "https://bob:" + "pw" + "@prom.example.com"

func TestASecretOnTheCommandLineNeverReachesTheBundle(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		want string
	}{
		{name: "a client secret", arg: "--auth-oidc-client-secret=hunter2", want: "--auth-oidc-client-secret=[redacted]"},
		{name: "a session secret", arg: "--session-secret=abc", want: "--session-secret=[redacted]"},
		{name: "a token file", arg: "--token-file=/run/token", want: "--token-file=[redacted]"},
		{name: "a webhook with a token in the query", arg: "--audit-webhook=https://hooks.example.com/x?token=abc", want: "--audit-webhook=https://hooks.example.com/x?[redacted]"},
		{name: "a url with credentials", arg: "--prometheus=" + urlWithLogin, want: "--prometheus=https://%5Bredacted%5D@prom.example.com"},
		{name: "an ordinary flag", arg: "--log-level=debug", want: "--log-level=debug"},
		{name: "a bare flag", arg: "--cluster-mode", want: "--cluster-mode"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			if got := redactArgument(one.arg); got != one.want {
				t.Fatalf("redacted = %q, want %q", got, one.want)
			}
		})
	}
}

func TestTheBundleSaysWhatItIsNotCarrying(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseSettings(settingsstore.Memory())

	bundle := srv.supportBundle()

	if len(bundle.Leftout) == 0 {
		t.Fatal("the bundle does not say what it leaves out")
	}
	if bundle.Version == "" || bundle.Go == "" || bundle.Platform == "" {
		t.Fatalf("bundle = %+v", bundle)
	}
}

func TestASettingIsDescribedRatherThanCopied(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseSettings(settingsstore.Memory())
	if err := srv.stored().Merge(map[string]string{"spinoza.theme.v1": "borg", "empty.v1": ""}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	bundle := srv.supportBundle()

	if bundle.Settings["spinoza.theme.v1"] != "set, under a kilobyte" {
		t.Fatalf("theme reads %q", bundle.Settings["spinoza.theme.v1"])
	}
	if bundle.Settings["empty.v1"] != "empty" {
		t.Fatalf("empty reads %q", bundle.Settings["empty.v1"])
	}
	for _, described := range bundle.Settings {
		if strings.Contains(described, "borg") {
			t.Fatal("a setting value was copied into the bundle")
		}
	}
}

func TestTheBundleDownloadsAsAFileAndParses(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseSettings(settingsstore.Memory())

	rec := httptest.NewRecorder()
	srv.handleSupport(rec, httptest.NewRequest(http.MethodGet, "/api/support", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "spinoza-support.json") {
		t.Fatalf("disposition = %q", rec.Header().Get("Content-Disposition"))
	}
	var read map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &read); err != nil {
		t.Fatalf("the bundle did not parse: %v", err)
	}
	if _, held := read["metrics"]; !held {
		t.Fatal("the bundle carries no metrics")
	}
}

func TestTheBundleCountsPeopleRatherThanNamingTheirDigests(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseSettings(settingsstore.Memory())
	err := srv.stored().Merge(map[string]string{
		userSettingsPrefix + "abc123.spinoza.theme.v1":  "borg",
		userSettingsPrefix + "def456.spinoza.theme.v1":  "mugen",
		userSettingsPrefix + "def456.spinoza.panels.v1": "{}",
		"spinoza.mutes.v1": "[]",
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	bundle := srv.supportBundle()

	for key := range bundle.Settings {
		if strings.Contains(key, "abc123") || strings.Contains(key, "def456") {
			t.Fatalf("the bundle names a person's digest: %q", key)
		}
	}
	if bundle.Settings["<people with settings of their own>"] != "2" {
		t.Fatalf("people = %q, want 2", bundle.Settings["<people with settings of their own>"])
	}
	if bundle.Settings["<somebody>.spinoza.theme.v1"] == "" {
		t.Fatal("the bundle dropped the personal settings entirely")
	}
	if bundle.Settings["spinoza.mutes.v1"] == "" {
		t.Fatal("the bundle dropped a deployment-wide setting")
	}
}

func TestAPersonalSettingWithNoNameIsStillDescribedWithoutNamingAnybody(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseSettings(settingsstore.Memory())
	if err := srv.stored().Merge(map[string]string{userSettingsPrefix + "abc123": "borg"}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	bundle := srv.supportBundle()

	if bundle.Settings[userSettingsPrefix+"<somebody>"] != "set, under a kilobyte" {
		t.Fatalf("settings = %+v, want the key described without the digest", bundle.Settings)
	}
	for key := range bundle.Settings {
		if strings.Contains(key, "abc123") {
			t.Fatalf("the bundle names a person's digest: %q", key)
		}
	}
}

func TestABigSettingIsDescribedBySizeRatherThanCopied(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseSettings(settingsstore.Memory())
	if err := srv.stored().Merge(map[string]string{"spinoza.themes.v1": strings.Repeat("x", 2048)}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	bundle := srv.supportBundle()

	if bundle.Settings["spinoza.themes.v1"] != "set, over a kilobyte" {
		t.Fatalf("themes reads %q", bundle.Settings["spinoza.themes.v1"])
	}
}

func TestAnArgumentThatLooksLikeAURLAndIsNotIsRedactedWhole(t *testing.T) {
	got := redactArgument("--public-url=https://[::1")

	if got != "--public-url="+redacted {
		t.Fatalf("argument = %q, want it redacted whole rather than half-parsed", got)
	}
}

func TestTheBundleCarriesTheMetricsPage(t *testing.T) {
	if !strings.Contains(supportMetrics(), "spinoza_") {
		t.Fatal("the bundle carried no metrics at all")
	}
}
