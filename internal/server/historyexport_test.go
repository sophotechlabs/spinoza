package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/store"
)

func auditExported() *heldHistory {
	return &heldHistory{
		page: store.Page{Entries: []store.Entry{{
			ID:        7,
			At:        recordedAt,
			Verb:      "delete",
			Actor:     "alice@example.com",
			Group:     "apps",
			Version:   "v1",
			Resource:  "deployments",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "web",
			Detail:    "deleted the deployment",
			Outcome:   api.HistoryDone,
		}}},
		changePage: store.Changes{Rows: []store.Change{{
			ID:        3,
			At:        recordedAt,
			Verb:      store.Changed,
			Version:   "v1",
			Resource:  "pods",
			Kind:      "Pod",
			Namespace: "default",
			Name:      "web-0",
			Cells:     []string{"1/1"},
		}}},
	}
}

func auditExportCSV(t *testing.T, body []byte) [][]string {
	t.Helper()
	rows, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("read the csv: %v: %s", err, body)
	}
	if len(rows) == 0 {
		t.Fatal("the export carried no rows at all")
	}
	return rows
}

func auditExportColumn(t *testing.T, rows [][]string, name string) int {
	t.Helper()
	for at, header := range rows[0] {
		if header == name {
			return at
		}
	}
	t.Fatalf("the header row has no %q column: %v", name, rows[0])
	return -1
}

func TestTheHistoryExportCarriesAHeaderAndARowPerEntry(t *testing.T) {
	ts := pastServer(t, auditExported())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "spinoza-history.csv") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	rows := auditExportCSV(t, body)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want a header and the two entries: %v", len(rows), rows)
	}
	verb := auditExportColumn(t, rows, "verb")
	name := auditExportColumn(t, rows, "name")
	if rows[1][verb] != "delete" || rows[1][name] != "web" {
		t.Fatalf("the action row was %v", rows[1])
	}
	if rows[2][name] != "web-0" {
		t.Fatalf("the change row was %v", rows[2])
	}
}

func TestTheHistoryExportNeutralizesSpreadsheetFormulas(t *testing.T) {
	held := auditExported()
	formula := `=HYPERLINK("https://attacker.example","Open")`
	held.page.Entries[0].Detail = formula
	ts := pastServer(t, held)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export", nil)

	rows := auditExportCSV(t, body)
	detail := auditExportColumn(t, rows, "detail")
	if rows[1][detail] == formula {
		t.Fatalf("exported an executable spreadsheet formula %q", rows[1][detail])
	}
	if rows[1][detail] != "'"+formula {
		t.Fatalf("detail = %q, want it prefixed so a spreadsheet reads it as text", rows[1][detail])
	}
}

func TestTheHistoryExportInJSONCarriesTheFieldsTheViewReceives(t *testing.T) {
	ts := pastServer(t, auditExported())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export?format=json", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "spinoza-history.json") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	var got []api.HistoryEntry
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %d, want the action and the change: %s", len(got), body)
	}
	if got[0].Verb != "delete" || got[0].Name != "web" || got[0].Source != api.HistoryAction {
		t.Fatalf("the action entry came back as %+v", got[0])
	}
	if got[0].At != "2026-08-29T09:30:00Z" {
		t.Fatalf("at = %q, want RFC3339 in UTC", got[0].At)
	}
}

func TestAnUnknownExportFormatFallsBackToCSV(t *testing.T) {
	ts := pastServer(t, auditExported())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export?format=pdf", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Fatalf("Content-Type = %q, want the csv the view asked for by default", got)
	}
	if len(auditExportCSV(t, body)) != 3 {
		t.Fatalf("an unknown format changed what came back: %s", body)
	}
}

func TestTheHistoryExportNarrowsTheSameWayTheViewDoes(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "everything by default", query: "", want: []string{"web", "web-0"}},
		{name: "actions only", query: "?source=action", want: []string{"web"}},
		{name: "changes only", query: "?source=change", want: []string{"web-0"}},
		{name: "a limit the store obeys", query: "?limit=1", want: []string{"web"}},
		{name: "past an action cursor", query: "?afterAction=1", want: []string{"web-0"}},
		{name: "past a change cursor", query: "?after=1", want: []string{"web"}},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			ts := pastServer(t, auditExported())

			_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export"+one.query, nil)

			rows := auditExportCSV(t, body)
			name := auditExportColumn(t, rows, "name")
			got := []string{}
			for _, row := range rows[1:] {
				got = append(got, row[name])
			}
			if strings.Join(got, ",") != strings.Join(one.want, ",") {
				t.Fatalf("exported %v, want %v", got, one.want)
			}
		})
	}
}

func TestTheHistoryExportIsScopedToTheConnectedClusterUnlessTheFleetIsAsked(t *testing.T) {
	for query, want := range map[string]string{"": stubClusterID, "?fleet=true": ""} {
		held := auditExported()
		ts := pastServer(t, held)

		doRequest(t, http.MethodGet, ts.URL+"/api/history/export"+query, nil)

		if got := held.asked().Cluster; got != want {
			t.Fatalf("export%q asked for cluster %q, want %q", query, got, want)
		}
	}
}

func TestAHistoryExportWithAFilterItCannotReadIsRefused(t *testing.T) {
	ts := pastServer(t, auditExported())

	for _, query := range []string{"?source=everything", "?limit=lots", "?limit=-1", "?after=soon", "?afterAction=-2"} {
		resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export"+query, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %s = %d, want 400: %s", query, resp.StatusCode, body)
		}
	}
}

func TestAHistoryExportSaysWhenNothingIsBeingRecorded(t *testing.T) {
	ts := pastServer(t, nil)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export", nil)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), api.HistoryOff) {
		t.Fatalf("body = %s, want it to say why there is nothing to export", body)
	}
}

func TestAHistoryExportThatCannotBeReadIsReported(t *testing.T) {
	ts := pastServer(t, &heldHistory{readErr: errBadLimit})

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("status = %d, want the failure reported rather than an empty file: %s", resp.StatusCode, body)
	}
}

func TestAnExportOfAnEmptyHistoryIsStillAHeaderRow(t *testing.T) {
	ts := pastServer(t, &heldHistory{})

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export", nil)

	rows := auditExportCSV(t, body)
	if len(rows) != 1 || rows[0][0] != "id" {
		t.Fatalf("an empty export was %v", rows)
	}
}

func TestAnExportHandsOverEveryPageAndNotJustTheFirst(t *testing.T) {
	held := &heldHistory{}
	for at := range exportPageSize + 250 {
		held.page.Entries = append(held.page.Entries, store.Entry{
			ID:     int64(exportPageSize + 250 - at),
			At:     time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC).Add(-time.Duration(at) * time.Second),
			Verb:   "delete",
			Actor:  "alice@example.com",
			Name:   "row-" + strconv.Itoa(at),
			Kind:   "ConfigMap",
			Detail: "",
		})
	}
	ts := pastServer(t, held)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export?source=action", nil)

	rows := auditExportCSV(t, body)
	if len(rows) != exportPageSize+250+1 {
		t.Fatalf("the export handed over %d rows, want every one of %d", len(rows)-1, exportPageSize+250)
	}
}

func manyActions(count int) *heldHistory {
	held := &heldHistory{}
	for at := range count {
		held.page.Entries = append(held.page.Entries, store.Entry{
			ID:    int64(count - at),
			At:    time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC).Add(-time.Duration(at) * time.Second),
			Verb:  "delete",
			Actor: "alice@example.com",
			Name:  "row-" + strconv.Itoa(at),
			Kind:  "ConfigMap",
		})
	}
	return held
}

func TestAnExportHandsOverNoMoreRowsThanWereAskedFor(t *testing.T) {
	held := manyActions(20)
	held.overDelivers = true
	ts := pastServer(t, held)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export?source=action&limit=5", nil)

	rows := auditExportCSV(t, body)
	if len(rows) != 6 {
		t.Fatalf("the export handed over %d rows, want the 5 that were asked for", len(rows)-1)
	}
}

func TestAnExportStopsWhenTheStoreKeepsHandingBackTheSamePage(t *testing.T) {
	held := manyActions(3)
	held.page.More = true
	held.stuckCursor = true
	ts := pastServer(t, held)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history/export?source=action", nil)

	rows := auditExportCSV(t, body)
	if len(rows) != 7 {
		t.Fatalf("the export handed over %d rows, want the page twice and then a stop", len(rows)-1)
	}
}
