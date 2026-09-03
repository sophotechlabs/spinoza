package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/settings"
)

type refusingSettings struct{}

func (refusingSettings) All() map[string]string {
	return map[string]string{}
}

func (refusingSettings) Off(string) bool {
	return false
}

func (refusingSettings) Merge(map[string]string) error {
	return errors.New("the settings file is read-only")
}

func settingsServer(t *testing.T, store Settings) *httptest.Server {
	t.Helper()
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	if store != nil {
		srv.UseSettings(store)
	}
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func settingsServerForUsers(t *testing.T, store Settings) (*Server, *httptest.Server) {
	t.Helper()
	srv := New(&stubBackendCluster{backend: everyNamespace()}, testAssets(), "")
	srv.UseSettings(store)
	authn, err := auth.New(t.Context(), auth.Config{
		Mode:        auth.ModeProxy,
		DefaultRole: auth.RoleViewer,
		Proxy: auth.ProxyConfig{
			SharedSecret: []byte(testProxySecret),
		},
	})
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	srv.UseClusterAuth(ClusterAuth{Authenticator: authn})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func settingsAsUser(
	t *testing.T,
	ts *httptest.Server,
	method string,
	path string,
	user string,
	body string,
) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set(auth.DefaultUserHeader, user)
	req.Header.Set(auth.DefaultProxyAuthHeader, testProxySecret)
	resp, doErr := ts.Client().Do(req)
	if doErr != nil {
		t.Fatalf("%s %s: %v", method, path, doErr)
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("reading response: %v", readErr)
	}
	return resp, responseBody
}

func putUserSettings(
	t *testing.T,
	ts *httptest.Server,
	user string,
	values map[string]string,
) (*http.Response, []byte) {
	t.Helper()
	body, err := json.Marshal(api.Settings{Values: values})
	if err != nil {
		t.Fatalf("encoding settings: %v", err)
	}
	return settingsAsUser(t, ts, http.MethodPut, "/api/settings", user, string(body))
}

func decodeSettings(t *testing.T, body []byte) api.Settings {
	t.Helper()
	var kept api.Settings
	err := json.Unmarshal(body, &kept)
	if err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return kept
}

func TestSettingsStartEmpty(t *testing.T) {
	ts := settingsServer(t, nil)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/settings", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(decodeSettings(t, body).Values) != 0 {
		t.Fatalf("a fresh server already holds %s", body)
	}
}

func TestSettingsComeBackAfterTheyAreKept(t *testing.T) {
	ts := settingsServer(t, settings.Memory())
	sent := `{"values":{"spinoza.theme.v1":"\"nord\""}}`

	put, putBody := doRequest(t, http.MethodPut, ts.URL+"/api/settings", strings.NewReader(sent))
	if put.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", put.StatusCode, putBody)
	}

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/settings", nil)

	if decodeSettings(t, body).Values["spinoza.theme.v1"] != `"nord"` {
		t.Fatalf("the theme did not survive: %s", body)
	}
}

func TestServedSettingsBelongToTheSignedInUser(t *testing.T) {
	store := settings.Memory()
	err := store.Merge(map[string]string{"spinoza.theme.v1": `"legacy"`})
	if err != nil {
		t.Fatalf("holding legacy settings: %v", err)
	}
	_, ts := settingsServerForUsers(t, store)

	_, first := settingsAsUser(t, ts, http.MethodGet, "/api/settings", "alice@example.com", "")
	if len(decodeSettings(t, first).Values) != 0 {
		t.Fatalf("alice inherited shared settings: %s", first)
	}
	alice, aliceBody := putUserSettings(t, ts, "alice@example.com", map[string]string{
		"spinoza.theme.v1": `"nord"`,
	})
	if alice.StatusCode != http.StatusOK {
		t.Fatalf("alice status = %d: %s", alice.StatusCode, aliceBody)
	}
	_, bobBefore := settingsAsUser(t, ts, http.MethodGet, "/api/settings", "bob@example.com", "")
	if len(decodeSettings(t, bobBefore).Values) != 0 {
		t.Fatalf("bob received alice's settings: %s", bobBefore)
	}
	bob, bobBody := putUserSettings(t, ts, "bob@example.com", map[string]string{
		"spinoza.theme.v1": `"solarized"`,
	})
	if bob.StatusCode != http.StatusOK {
		t.Fatalf("bob status = %d: %s", bob.StatusCode, bobBody)
	}

	_, aliceAfter := settingsAsUser(t, ts, http.MethodGet, "/api/settings", "alice@example.com", "")
	if decodeSettings(t, aliceAfter).Values["spinoza.theme.v1"] != `"nord"` {
		t.Fatalf("alice received the wrong settings: %s", aliceAfter)
	}
	_, bobAfter := settingsAsUser(t, ts, http.MethodGet, "/api/settings", "bob@example.com", "")
	if decodeSettings(t, bobAfter).Values["spinoza.theme.v1"] != `"solarized"` {
		t.Fatalf("bob received the wrong settings: %s", bobAfter)
	}
	stored := store.All()
	if stored["spinoza.theme.v1"] != `"legacy"` {
		t.Fatalf("personal settings overwrote the legacy shared value: %v", stored)
	}
	users := 0
	for key := range stored {
		if strings.Contains(key, "alice@example.com") || strings.Contains(key, "bob@example.com") {
			t.Fatalf("the settings store exposes a username in %q", key)
		}
		if strings.HasPrefix(key, userSettingsPrefix) {
			users++
		}
	}
	if users != 2 {
		t.Fatalf("stored user settings = %d, want 2: %v", users, stored)
	}
}

func TestPersonalSettingsAcceptTheAggregateLimitAndRejectTheNextByte(t *testing.T) {
	store := settings.Memory()
	_, ts := settingsServerForUsers(t, store)
	keys := []string{
		"spinoza.theme.v1",
		"spinoza.panels.v1",
		"spinoza.layout.v1",
		"spinoza.sidebar.v1",
	}
	keyBytes := 0
	for _, key := range keys {
		keyBytes += len(key)
	}
	remaining := maxPersonalSettingsBytes - keyBytes
	values := map[string]string{}
	for at, key := range keys {
		length := remaining / len(keys)
		if at < remaining%len(keys) {
			length++
		}
		values[key] = strings.Repeat("x", length)
	}
	accepted, body := putUserSettings(t, ts, "alice@example.com", values)
	if accepted.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", accepted.StatusCode, http.StatusOK, body)
	}
	values[keys[0]] += "x"
	rejected, body := putUserSettings(t, ts, "alice@example.com", values)
	if rejected.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rejected.StatusCode, http.StatusBadRequest)
	}
	if !strings.Contains(string(body), "personal settings are larger than 65536 bytes") {
		t.Fatalf("body = %s", body)
	}
}

func TestPersonalSettingSchemasHaveExactValueLimits(t *testing.T) {
	_, ts := settingsServerForUsers(t, settings.Memory())
	for _, tc := range []struct {
		name  string
		key   string
		limit int
	}{
		{name: "ordinary", key: "spinoza.theme.v1", limit: defaultPersonalSettingBytes},
		{name: "custom rules", key: checks.RulesKey, limit: personalSettingLimits[checks.RulesKey]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accepted, body := putUserSettings(t, ts, tc.name+"-accepted", map[string]string{
				tc.key: strings.Repeat("x", tc.limit),
			})
			if accepted.StatusCode != http.StatusOK {
				t.Fatalf("accepted status = %d: %s", accepted.StatusCode, body)
			}
			rejected, body := putUserSettings(t, ts, tc.name+"-rejected", map[string]string{
				tc.key: strings.Repeat("x", tc.limit+1),
			})
			if rejected.StatusCode != http.StatusBadRequest {
				t.Fatalf("rejected status = %d, want %d", rejected.StatusCode, http.StatusBadRequest)
			}
			if !strings.Contains(string(body), "is larger than") {
				t.Fatalf("body = %s", body)
			}
		})
	}
}

func TestOneViewerCannotConsumeAnotherUsersOrSharedSettingsCapacity(t *testing.T) {
	store := settings.Memory()
	_, ts := settingsServerForUsers(t, store)
	attacker, _ := putUserSettings(t, ts, "alice@example.com", map[string]string{
		"spinoza.theme.v1": strings.Repeat("x", 900<<10),
	})
	if attacker.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized viewer status = %d, want %d", attacker.StatusCode, http.StatusBadRequest)
	}
	bob, body := putUserSettings(t, ts, "bob@example.com", map[string]string{
		"spinoza.theme.v1": `"nord"`,
	})
	if bob.StatusCode != http.StatusOK {
		t.Fatalf("bob status = %d: %s", bob.StatusCode, body)
	}
	if err := store.Merge(map[string]string{checks.MutesKey: `{"cluster":[]}`}); err != nil {
		t.Fatalf("shared settings write: %v", err)
	}
}

func TestPersonalSettingsRequestHasAnExactByteLimit(t *testing.T) {
	_, ts := settingsServerForUsers(t, settings.Memory())
	prefix := `{"values":{}}`
	exact := prefix + strings.Repeat(" ", maxSettingsBytes-len(prefix))
	accepted, body := settingsAsUser(t, ts, http.MethodPut, "/api/settings", "alice", exact)
	if accepted.StatusCode != http.StatusOK {
		t.Fatalf("exact status = %d: %s", accepted.StatusCode, body)
	}
	rejected, _ := settingsAsUser(t, ts, http.MethodPut, "/api/settings", "alice", exact+" ")
	if rejected.StatusCode != http.StatusBadRequest {
		t.Fatalf("one extra byte status = %d, want %d", rejected.StatusCode, http.StatusBadRequest)
	}
}

func TestConcurrentPersonalWritesCannotBypassTheAggregateQuota(t *testing.T) {
	store := settings.Memory()
	_, ts := settingsServerForUsers(t, store)
	keys := []string{
		"spinoza.theme.v1",
		"spinoza.panels.v1",
		"spinoza.layout.v1",
		"spinoza.sidebar.v1",
		"spinoza.settings.v1",
		"spinoza.painted.v1",
		"spinoza.nodeshell.v1",
		"spinoza.update.check.v1",
	}
	start := make(chan struct{})
	var group sync.WaitGroup
	for _, key := range keys {
		group.Go(func() {
			<-start
			putUserSettings(t, ts, "alice@example.com", map[string]string{key: strings.Repeat("x", 12<<10)})
		})
	}
	close(start)
	group.Wait()
	prefix, ok := settingsPrefix(httptest.NewRequest(http.MethodGet, "/", http.NoBody).WithContext(
		auth.WithIdentity(t.Context(), auth.Identity{User: "alice@example.com"}),
	))
	if !ok {
		t.Fatal("alice had no settings prefix")
	}
	personal := map[string]string{}
	for key, value := range store.All() {
		personalKey, found := strings.CutPrefix(key, prefix)
		if found {
			personal[personalKey] = value
		}
	}
	if size := personalSettingsSize(personal); size > maxPersonalSettingsBytes {
		t.Fatalf("concurrent settings use %d bytes, want at most %d", size, maxPersonalSettingsBytes)
	}
}

func TestTheServedPageCarriesOnlyTheSignedInUsersSettings(t *testing.T) {
	_, ts := settingsServerForUsers(t, settings.Memory())
	putUserSettings(t, ts, "alice@example.com", map[string]string{"spinoza.theme.v1": `"nord"`})
	putUserSettings(t, ts, "bob@example.com", map[string]string{"spinoza.theme.v1": `"solarized"`})

	_, alicePage := settingsAsUser(t, ts, http.MethodGet, "/", "alice@example.com", "")
	if !strings.Contains(string(alicePage), "nord") {
		t.Fatalf("alice's page left out her settings: %s", alicePage)
	}
	if strings.Contains(string(alicePage), "solarized") {
		t.Fatalf("alice's page included bob's settings: %s", alicePage)
	}
	_, bobPage := settingsAsUser(t, ts, http.MethodGet, "/", "bob@example.com", "")
	if !strings.Contains(string(bobPage), "solarized") {
		t.Fatalf("bob's page left out his settings: %s", bobPage)
	}
	if strings.Contains(string(bobPage), "nord") {
		t.Fatalf("bob's page included alice's settings: %s", bobPage)
	}
}

func TestCustomCheckRulesBelongToTheSignedInUser(t *testing.T) {
	srv, ts := settingsServerForUsers(t, settings.Memory())
	raw := `[{"id":"alice-rule","expr":"true"}]`
	resp, body := putUserSettings(t, ts, "alice@example.com", map[string]string{checks.RulesKey: raw})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	alice := httptest.NewRequest(http.MethodGet, "/api/checks", http.NoBody)
	alice = alice.WithContext(auth.WithIdentity(alice.Context(), auth.Identity{User: "alice@example.com"}))
	bob := httptest.NewRequest(http.MethodGet, "/api/checks", http.NoBody)
	bob = bob.WithContext(auth.WithIdentity(bob.Context(), auth.Identity{User: "bob@example.com"}))

	aliceRules := srv.checkFilterOn(alice, "cluster").Rules
	if len(aliceRules) != 1 || aliceRules[0].ID != "alice-rule" {
		t.Fatalf("alice's rules = %+v", aliceRules)
	}
	bobRules := srv.checkFilterOn(bob, "cluster").Rules
	if len(bobRules) != 0 {
		t.Fatalf("bob received alice's rules: %+v", bobRules)
	}
}

func TestServedSettingsAcceptOnlyBrowserManagedKeys(t *testing.T) {
	_, ts := settingsServerForUsers(t, settings.Memory())
	resp, _ := putUserSettings(t, ts, "alice@example.com", map[string]string{
		"spinoza.unbounded.v1": "value",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestTheBrowserAndServerAgreeOnPersonalSettingKeys(t *testing.T) {
	body, err := os.ReadFile("../../frontend/src/lib/persist.ts")
	if err != nil {
		t.Fatalf("reading browser settings: %v", err)
	}
	source := string(body)
	start := strings.Index(source, "const KEYS = [")
	if start < 0 {
		t.Fatal("the browser settings list could not be found")
	}
	end := strings.Index(source[start:], "];\n")
	if end < 0 {
		t.Fatal("the end of the browser settings list could not be found")
	}
	block := source[start : start+end]
	browser := map[string]bool{}
	for line := range strings.SplitSeq(block, "\n") {
		key := strings.Trim(strings.TrimSpace(line), "',")
		if strings.HasPrefix(key, "spinoza.") {
			browser[key] = true
		}
	}
	if len(browser) != len(personalSettingKeys) {
		t.Fatalf("browser keys = %v, server keys = %v", browser, personalSettingKeys)
	}
	for key := range browser {
		if !personalSettingKeys[key] {
			t.Errorf("the browser writes %q but the server refuses it", key)
		}
	}
}

func TestMutesDoNotComeBackThroughSettings(t *testing.T) {
	store := settings.Memory()
	err := store.Merge(map[string]string{
		"spinoza.theme.v1": `"nord"`,
		checks.MutesKey:    `{"cluster":[{"check":"privileged","namespace":"payments"}]}`,
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	ts := settingsServer(t, store)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/settings", nil)
	values := decodeSettings(t, body).Values

	if _, exposed := values[checks.MutesKey]; exposed {
		t.Fatalf("settings exposed check mutes: %s", body)
	}
	if values["spinoza.theme.v1"] != `"nord"` {
		t.Fatalf("ordinary settings were filtered with the mutes: %s", body)
	}
}

func TestDeploymentAndOtherUsersSettingsDoNotComeBackThroughSettings(t *testing.T) {
	store := settings.Memory()
	otherUser := userSettingsPrefix + strings.Repeat("a", 64) + ".spinoza.theme.v1"
	err := store.Merge(map[string]string{
		"spinoza.theme.v1": `"nord"`,
		timelineDaysKey:    "30",
		otherUser:          `"solarized"`,
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	ts := settingsServer(t, store)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/settings", nil)
	values := decodeSettings(t, body).Values

	if values["spinoza.theme.v1"] != `"nord"` {
		t.Fatalf("ordinary settings were filtered: %s", body)
	}
	_, exposedTimeline := values[timelineDaysKey]
	if exposedTimeline {
		t.Fatalf("settings exposed timeline retention: %s", body)
	}
	_, exposedUser := values[otherUser]
	if exposedUser {
		t.Fatalf("settings exposed another user's values: %s", body)
	}
}

func TestMutesCannotBeChangedThroughSettings(t *testing.T) {
	store := settings.Memory()
	original := `{"cluster":[{"check":"privileged","namespace":"payments"}]}`
	if err := store.Merge(map[string]string{checks.MutesKey: original}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	ts := settingsServer(t, store)
	body := `{"values":{"` + checks.MutesKey + `":"[]"}}`

	resp, _ := doRequest(t, http.MethodPut, ts.URL+"/api/settings", strings.NewReader(body))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if store.All()[checks.MutesKey] != original {
		t.Fatal("the settings endpoint changed the shared mute state")
	}
}

func TestDeploymentSettingsCannotBeChangedThroughPersonalSettings(t *testing.T) {
	ts := settingsServer(t, settings.Memory())
	for _, key := range []string{
		timelineDaysKey,
		userSettingsPrefix + strings.Repeat("a", 64) + ".spinoza.theme.v1",
	} {
		t.Run(key, func(t *testing.T) {
			body, err := json.Marshal(api.Settings{Values: map[string]string{key: "value"}})
			if err != nil {
				t.Fatalf("encoding settings: %v", err)
			}
			resp, _ := doRequest(t, http.MethodPut, ts.URL+"/api/settings", strings.NewReader(string(body)))
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestKeepingSettingsAnswersWithWhatWasKept(t *testing.T) {
	ts := settingsServer(t, settings.Memory())

	_, body := doRequest(
		t,
		http.MethodPut,
		ts.URL+"/api/settings",
		strings.NewReader(`{"values":{"a":"1"}}`),
	)

	if decodeSettings(t, body).Values["a"] != "1" {
		t.Fatalf("the answer left it out: %s", body)
	}
}

func TestSettingsThatAreNotAnObjectAreRefused(t *testing.T) {
	ts := settingsServer(t, settings.Memory())

	resp, _ := doRequest(t, http.MethodPut, ts.URL+"/api/settings", strings.NewReader("nonsense"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSettingsRefuseASecondJSONValue(t *testing.T) {
	store := settings.Memory()
	ts := settingsServer(t, store)
	body := `{"values":{"a":"1"}} {"values":{"a":"2"}}`

	resp, _ := doRequest(t, http.MethodPut, ts.URL+"/api/settings", strings.NewReader(body))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(store.All()) != 0 {
		t.Fatalf("settings changed to %v", store.All())
	}
}

func TestJSONBodiesEnforceTheirByteLimitAfterACompleteValue(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{} "))
	recorder := httptest.NewRecorder()
	var value map[string]any

	err := decodeJSONBody(recorder, request, 2, &value)

	var tooBig *http.MaxBytesError
	if !errors.As(err, &tooBig) {
		t.Fatalf("err = %v, want a body-size error", err)
	}
}

func TestSettingsThatCannotBeWrittenAreReported(t *testing.T) {
	ts := settingsServer(t, refusingSettings{})

	resp, body := doRequest(
		t,
		http.MethodPut,
		ts.URL+"/api/settings",
		strings.NewReader(`{"values":{"a":"1"}}`),
	)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if !strings.Contains(string(body), "read-only") {
		t.Fatalf("the refusal does not say why: %s", body)
	}
}

func TestThePageCarriesTheSettingsWithIt(t *testing.T) {
	store := settings.Memory()
	err := store.Merge(map[string]string{"spinoza.theme.v1": `"nord"`})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	ts := settingsServer(t, store)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "__SPINOZA_SETTINGS__") {
		t.Fatalf("the page carries no settings: %s", body)
	}
	if !strings.Contains(string(body), `spinoza.theme.v1`) {
		t.Fatalf("the page left the theme out: %s", body)
	}
}

func TestTheSettingsScriptIsSafeToEmbed(t *testing.T) {
	script := SettingsScript(map[string]string{"a": `</script><script>alert(1)</script>`})

	if strings.Contains(script, "</script><script>") {
		t.Fatalf("the script can be broken out of: %s", script)
	}
}

func TestSettingsAreOfferedWithoutACluster(t *testing.T) {
	srv := New(fixed(nil), testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/settings", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the settings even with no cluster", resp.StatusCode)
	}
}
