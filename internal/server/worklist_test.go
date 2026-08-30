package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

type heldBaselines struct {
	taken   map[string]checks.Baseline
	saveErr error
	clearIt error
}

func newHeldBaselines() *heldBaselines {
	return &heldBaselines{taken: map[string]checks.Baseline{}}
}

func (h *heldBaselines) Load(cluster string) (checks.Baseline, bool) {
	found, ok := h.taken[cluster]
	return found, ok
}

func (h *heldBaselines) Save(cluster string, taken checks.Baseline) error {
	if h.saveErr != nil {
		return h.saveErr
	}
	h.taken[cluster] = taken
	return nil
}

func (h *heldBaselines) Clear(cluster string) error {
	if h.clearIt != nil {
		return h.clearIt
	}
	delete(h.taken, cluster)
	return nil
}

func send(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	encoded := []byte(nil)
	if body != nil {
		written, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode %v: %v", body, err)
		}
		encoded = written
	}
	req, reqErr := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(encoded))
	if reqErr != nil {
		t.Fatalf("request %s: %v", url, reqErr)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func bodyOf(t *testing.T, resp *http.Response, into any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func checksReport(t *testing.T, url string) api.CheckReport {
	t.Helper()
	var found api.CheckReport
	getJSON(t, url, &found)
	return found
}

func groupIn(t *testing.T, report api.CheckReport, id string) api.CheckGroup {
	t.Helper()
	for _, group := range report.Groups {
		if group.ID == id {
			return group
		}
	}
	t.Fatalf("no check named %q in the report", id)
	return api.CheckGroup{}
}

// muting through the endpoint

func TestAMuteSilencesTheFindingOnTheNextAudit(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	before := groupIn(t, checksReport(t, ts.URL+"/api/checks"), "requests-missing").Total

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/mutes", api.Mute{
		Check: "requests-missing", Ref: "/v1/pods/prod/web-0", Reason: "it is a one-off",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	after := groupIn(t, checksReport(t, ts.URL+"/api/checks"), "requests-missing")
	if after.Total != before-1 {
		t.Fatalf("the audit reported %d findings, want one fewer than %d", after.Total, before)
	}
	if after.Muted != 1 {
		t.Fatalf("the audit counted %d muted", after.Muted)
	}
}

func TestAMuteIsRememberedWithWhoeverAskedForItsReason(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	send(t, http.MethodPost, ts.URL+"/api/checks/mutes", api.Mute{
		Check: "requests-missing", Namespace: "prod", Reason: "prod is exempt",
	})

	var held api.Mutes
	getJSON(t, ts.URL+"/api/checks/mutes", &held)

	if len(held.Mutes) != 1 {
		t.Fatalf("the store holds %v", held.Mutes)
	}
	if held.Mutes[0].Reason != "prod is exempt" || held.Mutes[0].At == "" {
		t.Fatalf("the mute was kept as %+v, want the reason and the day it was made", held.Mutes[0])
	}
}

func TestMutingTheSameThingTwiceKeepsOneMute(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	one := api.Mute{Check: "requests-missing", Namespace: "prod", Reason: "first"}

	send(t, http.MethodPost, ts.URL+"/api/checks/mutes", one)
	one.Reason = "second"
	resp := send(t, http.MethodPost, ts.URL+"/api/checks/mutes", one)

	var held api.Mutes
	bodyOf(t, resp, &held)
	if len(held.Mutes) != 1 || held.Mutes[0].Reason != "second" {
		t.Fatalf("the store holds %v", held.Mutes)
	}
}

func TestUnmutingBringsTheFindingBack(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	mute := api.Mute{Check: "requests-missing", Ref: "/v1/pods/prod/web-0"}
	send(t, http.MethodPost, ts.URL+"/api/checks/mutes", mute)

	resp := send(t, http.MethodDelete, ts.URL+"/api/checks/mutes", mute)

	var held api.Mutes
	bodyOf(t, resp, &held)
	if len(held.Mutes) != 0 {
		t.Fatalf("the store still holds %v", held.Mutes)
	}
	if groupIn(t, checksReport(t, ts.URL+"/api/checks"), "requests-missing").Muted != 0 {
		t.Fatal("a finding stayed muted after being unmuted")
	}
}

func TestAMuteThatNamesNoCheckIsRefused(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/mutes", api.Mute{Namespace: "prod"})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSomethingThatIsNotAMuteIsRefused(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/mutes", "not an object")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// the baseline through the endpoint

func TestABaselineTakenNowLeavesNothingNew(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	srv.UseBaselines(newHeldBaselines())

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/baseline", nil)

	var taken api.Baseline
	bodyOf(t, resp, &taken)
	if taken.TakenAt == "" || taken.Findings == 0 {
		t.Fatalf("the baseline came back as %+v", taken)
	}
	group := groupIn(t, checksReport(t, ts.URL+"/api/checks"), "requests-missing")
	if group.NewCount != 0 || !group.Baselined {
		t.Fatalf("a baseline taken just now reported %d new (covered: %v)", group.NewCount, group.Baselined)
	}
}

func TestForgettingTheBaselineStopsAnythingBeingMarked(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	srv.UseBaselines(newHeldBaselines())
	send(t, http.MethodPost, ts.URL+"/api/checks/baseline", nil)

	send(t, http.MethodDelete, ts.URL+"/api/checks/baseline", nil)

	if report := checksReport(t, ts.URL+"/api/checks"); report.Baseline != "" {
		t.Fatalf("the report still names %q as its baseline", report.Baseline)
	}
}

func TestABaselineThatCannotBeKeptIsReported(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	held := newHeldBaselines()
	held.saveErr = errors.New("the disk is full")
	srv.UseBaselines(held)

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/baseline", nil)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func TestABaselineThatCannotBeForgottenIsReported(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	held := newHeldBaselines()
	held.clearIt = errors.New("it is read only")
	srv.UseBaselines(held)

	resp := send(t, http.MethodDelete, ts.URL+"/api/checks/baseline", nil)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func TestAServerGivenNoBaselineStoreStillTakesOne(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/baseline", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if report := checksReport(t, ts.URL+"/api/checks"); report.Baseline != "" {
		t.Fatalf("a server keeping no baselines named one: %q", report.Baseline)
	}
}

// the namespace summary through the endpoint

func TestTheReportCountsFindingsPerNamespace(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"), newPodObject("staging", "web-1"))

	report := checksReport(t, ts.URL+"/api/checks")

	spaces := map[string]int{}
	for _, entry := range report.Namespaces {
		spaces[entry.Namespace] = entry.Total
	}
	if spaces["prod"] == 0 || spaces["staging"] == 0 {
		t.Fatalf("the summary carried %v", report.Namespaces)
	}
}

func TestOneNamespaceCanBeAskedForThroughTheEndpoint(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"), newPodObject("staging", "web-1"))

	report := checksReport(t, ts.URL+"/api/checks?namespace=prod")

	for _, object := range report.Objects {
		if object.Namespace != "prod" {
			t.Fatalf("asking for prod returned an object in %s", object.Namespace)
		}
	}
	if len(report.Objects) == 0 {
		t.Fatal("asking for one namespace returned nothing at all")
	}
}
