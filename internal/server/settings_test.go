package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
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
