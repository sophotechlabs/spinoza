package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/settings"
	"github.com/sophotechlabs/spinoza/internal/update"
)

type stubUpdates struct {
	status    api.UpdateStatus
	asked     int
	rechecked int
}

func (s *stubUpdates) Status(context.Context) api.UpdateStatus {
	s.asked++
	return s.status
}

func (s *stubUpdates) Recheck(context.Context) api.UpdateStatus {
	s.rechecked++
	return s.status
}

type stubInstaller struct {
	err   error
	tries int
}

func (s *stubInstaller) Install(context.Context) error {
	s.tries++
	return s.err
}

func updateServer(t *testing.T, checker Updates) *httptest.Server {
	t.Helper()
	return updateServerWith(t, checker, nil, settings.Memory())
}

func updateServerWith(
	t *testing.T,
	checker Updates,
	installer Installs,
	store Settings,
) *httptest.Server {
	t.Helper()
	srv := New(nil, testAssets(), testToken)
	srv.UseSettings(store)
	if checker != nil {
		srv.UseUpdates(checker)
	}
	if installer != nil {
		srv.UseInstaller(installer)
	}
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func postUpdate(t *testing.T, url string) api.UpdateResult {
	t.Helper()
	res, err := http.Post(url+"/api/update", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var result api.UpdateResult
	if decodeErr := json.NewDecoder(res.Body).Decode(&result); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	return result
}

func newerRelease() *stubUpdates {
	return &stubUpdates{status: api.UpdateStatus{
		Checked:   true,
		Current:   "v1.14.1",
		Latest:    "v1.15.0",
		Available: true,
	}}
}

func readUpdate(t *testing.T, url string) api.UpdateStatus {
	t.Helper()
	res, err := http.Get(url + "/api/update")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var status api.UpdateStatus
	if decodeErr := json.NewDecoder(res.Body).Decode(&status); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	return status
}

func TestTheUpdateEndpointSaysWhatTheCheckerFound(t *testing.T) {
	checker := &stubUpdates{status: api.UpdateStatus{
		Checked:   true,
		Current:   "v1.14.1",
		Latest:    "v1.15.0",
		Available: true,
		Command:   "curl -fsSL https://spinoza.tech/install.sh | sh",
	}}
	ts := updateServer(t, checker)

	status := readUpdate(t, ts.URL)

	if !status.Available || status.Latest != "v1.15.0" {
		t.Fatalf("status = %+v, want what the checker found", status)
	}
	if checker.asked != 1 {
		t.Fatalf("checker asked %d times, want once per request", checker.asked)
	}
}

func TestTheUpdateEndpointAnswersWithoutAChecker(t *testing.T) {
	ts := updateServer(t, nil)

	status := readUpdate(t, ts.URL)

	if status.Available || status.Checked {
		t.Fatalf("status = %+v, want nothing claimed", status)
	}
	if status.Reason == "" {
		t.Fatal("no reason was given for there being no answer")
	}
}

func TestTheButtonInstallsWhatItFinds(t *testing.T) {
	checker := newerRelease()
	installer := &stubInstaller{}
	ts := updateServerWith(t, checker, installer, settings.Memory())

	result := postUpdate(t, ts.URL)

	if !result.Updated {
		t.Fatalf("result = %+v, want the update to have been installed", result)
	}
	if result.Latest != "v1.15.0" {
		t.Fatalf("latest = %q, want the release it installed", result.Latest)
	}
	if installer.tries != 1 {
		t.Fatalf("installed %d times, want once", installer.tries)
	}
}

// The button asks again rather than reading what the window was told on open.
func TestTheButtonAsksAgain(t *testing.T) {
	checker := newerRelease()
	ts := updateServerWith(t, checker, &stubInstaller{}, settings.Memory())

	postUpdate(t, ts.URL)

	if checker.rechecked != 1 {
		t.Fatalf("rechecked %d times, want once", checker.rechecked)
	}
	if checker.asked != 0 {
		t.Fatal("the button read the cached answer")
	}
}

func TestTheButtonSaysWhenThereIsNothingNewer(t *testing.T) {
	checker := &stubUpdates{status: api.UpdateStatus{
		Checked: true,
		Current: "v1.15.0",
		Latest:  "v1.15.0",
	}}
	installer := &stubInstaller{}
	ts := updateServerWith(t, checker, installer, settings.Memory())

	result := postUpdate(t, ts.URL)

	if result.Updated {
		t.Fatal("a build already on the newest release was updated anyway")
	}
	if installer.tries != 0 {
		t.Fatal("the installer ran with nothing to install")
	}
	if result.Latest != "v1.15.0" {
		t.Fatalf("latest = %q, want the release it is already on", result.Latest)
	}
}

// A desktop build has no installer wired up, so the button hands over the
// command instead of pretending it cannot be done at all.
func TestABuildThatCannotReplaceItselfOffersTheCommand(t *testing.T) {
	ts := updateServerWith(t, newerRelease(), nil, settings.Memory())

	result := postUpdate(t, ts.URL)

	if result.Updated {
		t.Fatal("a build with no installer reported an update")
	}
	if result.Command != update.Command {
		t.Fatalf("command = %q, want the install line", result.Command)
	}
}

func TestAFailedInstallComesBackWithWhatWentWrong(t *testing.T) {
	installer := &stubInstaller{err: errors.New("checksum did not match")}
	ts := updateServerWith(t, newerRelease(), installer, settings.Memory())

	result := postUpdate(t, ts.URL)

	if result.Updated {
		t.Fatal("a failed install was reported as done")
	}
	if result.Reason != "checksum did not match" {
		t.Fatalf("reason = %q, want what the installer said", result.Reason)
	}
	if result.Command != "" {
		t.Fatalf("command = %q, want none for a failure that is not about support", result.Command)
	}
}

// An install refused for what this build is comes with the command, so there is
// somewhere to go from there.
func TestAnUnsupportedInstallOffersTheCommand(t *testing.T) {
	installer := &stubInstaller{err: fmt.Errorf("%w: /usr/local/bin is not writable", update.ErrUnsupported)}
	ts := updateServerWith(t, newerRelease(), installer, settings.Memory())

	result := postUpdate(t, ts.URL)

	if result.Command != update.Command {
		t.Fatalf("command = %q, want the install line", result.Command)
	}
	if !strings.Contains(result.Reason, "not writable") {
		t.Fatalf("reason = %q, want what stood in the way", result.Reason)
	}
}

func TestTheButtonWithoutAChecker(t *testing.T) {
	ts := updateServerWith(t, nil, &stubInstaller{}, settings.Memory())

	if result := postUpdate(t, ts.URL); result.Reason == "" {
		t.Fatal("a build with no checker said nothing about why")
	}
}

// Turning the check off stops the automatic one. The button is a separate act.
func TestTurningTheCheckOffStopsTheAutomaticOne(t *testing.T) {
	store := settings.Memory()
	if err := store.Merge(map[string]string{settings.UpdateCheckKey: "off"}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	checker := newerRelease()
	ts := updateServerWith(t, checker, &stubInstaller{}, store)

	status := readUpdate(t, ts.URL)

	if status.Available || status.Checked {
		t.Fatalf("status = %+v, want nothing claimed", status)
	}
	if checker.asked != 0 {
		t.Fatal("the check ran despite being turned off")
	}
	if !strings.Contains(status.Reason, "turned off") {
		t.Fatalf("reason = %q, want it to name the setting", status.Reason)
	}
}

func TestTheButtonWorksWithTheAutomaticCheckOff(t *testing.T) {
	store := settings.Memory()
	if err := store.Merge(map[string]string{settings.UpdateCheckKey: "off"}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	installer := &stubInstaller{}
	ts := updateServerWith(t, newerRelease(), installer, store)

	if result := postUpdate(t, ts.URL); !result.Updated {
		t.Fatalf("result = %+v, want the button to work regardless", result)
	}
}
