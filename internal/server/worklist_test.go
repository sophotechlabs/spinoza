package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

type heldBaselines struct {
	taken   map[string]checks.Baseline
	saveErr error
	clearIt error
}

type brokenBaselineBody struct{}

func (brokenBaselineBody) Read([]byte) (int, error) {
	return 0, errors.New("upload interrupted")
}

func (brokenBaselineBody) Close() error {
	return nil
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

func sendBody(t *testing.T, method, url string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request %s: %v", url, err)
	}
	resp, sendErr := http.DefaultClient.Do(req)
	if sendErr != nil {
		t.Fatalf("%s %s: %v", method, url, sendErr)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func getRaw(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("request %s: %v", url, err)
	}
	resp, sendErr := http.DefaultClient.Do(req)
	if sendErr != nil {
		t.Fatalf("GET %s: %v", url, sendErr)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func readCSV(t *testing.T, resp *http.Response) [][]string {
	t.Helper()
	rows, err := csv.NewReader(resp.Body).ReadAll()
	if err != nil {
		t.Fatalf("read the csv: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("the export carried no rows at all")
	}
	return rows
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

func TestAServerGivenNoBaselineStoreStillClearsOne(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	resp := send(t, http.MethodDelete, ts.URL+"/api/checks/baseline", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestARuleListThatReadsComesBackWithNoFaults(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	rules := `[{"id":"no-beta","expr":"object.kind == \"Pod\""}]`

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/rules/faults", json.RawMessage(rules))

	var found api.RuleFaults
	bodyOf(t, resp, &found)
	if len(found.Faults) != 0 {
		t.Fatalf("a rule that compiles came back as %v", found.Faults)
	}
}

func TestARuleThatDoesNotCompileComesBackNamed(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	rules := `[{"id":"broken","expr":"object.spec.nope("}]`

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/rules/faults", json.RawMessage(rules))

	var found api.RuleFaults
	bodyOf(t, resp, &found)
	if len(found.Faults) != 1 || found.Faults[0].ID != "broken" {
		t.Fatalf("came back as %v", found.Faults)
	}
}

func TestARuleListTooLargeToReadIsRefused(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))
	huge := strings.Repeat("x", maxRulesBytes+1)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		ts.URL+"/api/checks/rules/faults", strings.NewReader(huge))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, sendErr := http.DefaultClient.Do(req)
	if sendErr != nil {
		t.Fatalf("post: %v", sendErr)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestTheExportCarriesEveryFindingAsARow(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"))

	resp := getRaw(t, ts.URL+"/api/checks/export")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "spinoza-checks.csv") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	rows := readCSV(t, resp)
	if rows[0][0] != "check" || rows[0][8] != "detail" {
		t.Fatalf("the header row was %v", rows[0])
	}
	found := false
	for _, row := range rows[1:] {
		if row[0] == "requests-missing" && row[6] == "web-0" && row[5] == "prod" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no row for the pod the audit reported, in %d rows", len(rows)-1)
	}
}

func TestTheExportObeysTheFilterTheViewIsShowing(t *testing.T) {
	ts, _ := dashboardPair(t, newPodObject("prod", "web-0"), newPodObject("staging", "web-1"))

	rows := readCSV(t, getRaw(t, ts.URL+"/api/checks/export?namespace=prod"))

	for _, row := range rows[1:] {
		if row[5] != "prod" {
			t.Fatalf("a row for %s came back from an export of prod", row[5])
		}
	}
	if len(rows) < 2 {
		t.Fatal("an export of one namespace carried nothing at all")
	}
}

func TestTheBaselineCanBeSavedToAFileAndTakenBack(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	srv.UseBaselines(newHeldBaselines())
	send(t, http.MethodPost, ts.URL+"/api/checks/baseline", nil)

	resp := getRaw(t, ts.URL+"/api/checks/baseline/file")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "spinoza-baseline.json") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	send(t, http.MethodDelete, ts.URL+"/api/checks/baseline", nil)
	back := sendBody(t, http.MethodPut, ts.URL+"/api/checks/baseline/file", body)

	if back.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", back.StatusCode)
	}
	var taken api.Baseline
	bodyOf(t, back, &taken)
	if taken.TakenAt == "" || taken.Findings == 0 {
		t.Fatalf("the baseline came back as %+v", taken)
	}
	if report := checksReport(t, ts.URL+"/api/checks"); report.Baseline != taken.TakenAt {
		t.Fatalf("the report names %q as its baseline", report.Baseline)
	}
}

func TestABaselineTakenHereNamesThisCluster(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	srv.UseBaselines(newHeldBaselines())

	resp := send(t, http.MethodPost, ts.URL+"/api/checks/baseline", nil)

	var taken api.Baseline
	bodyOf(t, resp, &taken)
	if taken.Cluster == "" {
		t.Fatal("a baseline taken here does not say which cluster it came from")
	}
}

func TestSavingABaselineNobodyTookIsRefused(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	srv.UseBaselines(newHeldBaselines())

	resp := getRaw(t, ts.URL+"/api/checks/baseline/file")

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSavingAnInvalidHeldBaselineIsReported(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	held := newHeldBaselines()
	srv.UseBaselines(held)
	send(t, http.MethodPost, ts.URL+"/api/checks/baseline", nil)
	for cluster := range held.taken {
		held.taken[cluster] = checks.Baseline{}
	}

	resp := getRaw(t, ts.URL+"/api/checks/baseline/file")

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func TestAFileThatIsNotABaselineIsRefused(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	srv.UseBaselines(newHeldBaselines())

	resp := sendBody(t, http.MethodPut, ts.URL+"/api/checks/baseline/file", []byte("{{{"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if report := checksReport(t, ts.URL+"/api/checks"); report.Baseline != "" {
		t.Fatalf("a refused file left %q as the baseline", report.Baseline)
	}
}

func TestABaselineUploadThatBreaksWhileReadingIsRefused(t *testing.T) {
	srv := New(&stubBackendCluster{}, testAssets(), testToken)
	held := newHeldBaselines()
	srv.UseBaselines(held)
	request := httptest.NewRequest(http.MethodPut, "/api/checks/baseline/file", http.NoBody)
	request.Body = brokenBaselineBody{}
	recorded := httptest.NewRecorder()

	srv.loadBaselineFile(recorded, request)

	if recorded.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorded.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorded.Body.String(), "could not be read") {
		t.Fatalf("body = %q, want the interrupted upload named", recorded.Body.String())
	}
	if len(held.taken) != 0 {
		t.Fatalf("baselines = %v, want the partial upload left out", held.taken)
	}
}

func TestABaselineThatCannotBeKeptOnLoadIsReported(t *testing.T) {
	ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
	held := newHeldBaselines()
	srv.UseBaselines(held)
	send(t, http.MethodPost, ts.URL+"/api/checks/baseline", nil)
	body, err := io.ReadAll(getRaw(t, ts.URL+"/api/checks/baseline/file").Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	held.saveErr = errors.New("the disk is full")

	resp := sendBody(t, http.MethodPut, ts.URL+"/api/checks/baseline/file", body)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

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
