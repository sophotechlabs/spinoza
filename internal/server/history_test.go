package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/store"
)

var recordedAt = time.Date(2026, 8, 29, 9, 30, 0, 0, time.UTC)

const stubClusterID = "https://p-mk1:6443"

type heldHistory struct {
	mu        sync.Mutex
	entries   []store.Entry
	page      store.Page
	reason    string
	lastQuery store.Query
	recordErr error
	readErr   error
	forgetErr error
	forgotten int

	forgotCluster string

	changes     []store.Change
	changePage  store.Changes
	changeErr   error
	pruned      []store.Retention
	prunedAudit []store.Retention
}

func (h *heldHistory) For(cluster string) store.Recorder {
	return heldWriter{into: h, cluster: cluster}
}

type heldWriter struct {
	into    *heldHistory
	cluster string
}

func (w heldWriter) Record(_ context.Context, entry store.Entry) error {
	held := w.into
	held.mu.Lock()
	defer held.mu.Unlock()
	if held.recordErr != nil {
		return held.recordErr
	}
	entry.Cluster = w.cluster
	held.entries = append(held.entries, entry)
	return nil
}

func (h *heldHistory) Timeline(cluster string) store.Noter {
	return heldNoter{into: h, cluster: cluster}
}

type heldNoter struct {
	into    *heldHistory
	cluster string
}

func (w heldNoter) Note(_ context.Context, changes []store.Change) error {
	held := w.into
	held.mu.Lock()
	defer held.mu.Unlock()
	if held.recordErr != nil {
		return held.recordErr
	}
	for _, one := range changes {
		one.Cluster = w.cluster
		held.changes = append(held.changes, one)
	}
	return nil
}

func (h *heldHistory) Changed(_ context.Context, query store.Query) (store.Changes, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastQuery = query
	if h.changeErr != nil {
		return store.Changes{}, h.changeErr
	}
	return store.Changes{Rows: rowsBelow(h.changePage.Rows, query.After), More: h.changePage.More}, nil
}

func rowsBelow(rows []store.Change, after int64) []store.Change {
	if after == 0 {
		return rows
	}
	out := make([]store.Change, 0, len(rows))
	for _, one := range rows {
		if one.ID < after {
			out = append(out, one)
		}
	}
	return out
}

func (h *heldHistory) Prune(_ context.Context, keep store.Retention, _ time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruned = append(h.pruned, keep)
	return nil
}

func (h *heldHistory) PruneAudit(_ context.Context, keep store.Retention, _ time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prunedAudit = append(h.prunedAudit, keep)
	return nil
}

func (h *heldHistory) noted() []store.Change {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]store.Change{}, h.changes...)
}

func (h *heldHistory) trims() []store.Retention {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]store.Retention{}, h.pruned...)
}

func (h *heldHistory) auditTrims() []store.Retention {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]store.Retention{}, h.prunedAudit...)
}

func (h *heldHistory) Recent(_ context.Context, query store.Query) (store.Page, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastQuery = query
	if h.readErr != nil {
		return store.Page{}, h.readErr
	}
	return store.Page{Entries: below(h.page.Entries, query.AfterAction), More: h.page.More}, nil
}

func below(entries []store.Entry, after int64) []store.Entry {
	if after == 0 {
		return entries
	}
	out := make([]store.Entry, 0, len(entries))
	for _, one := range entries {
		if one.ID < after {
			out = append(out, one)
		}
	}
	return out
}

func (h *heldHistory) asked() store.Query {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastQuery
}

func (h *heldHistory) Forget(_ context.Context, cluster string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.forgotCluster = cluster
	if h.forgetErr != nil {
		return h.forgetErr
	}
	h.forgotten++
	h.entries = kept(h.entries, cluster)
	return nil
}

func kept(held []store.Entry, cluster string) []store.Entry {
	if cluster == "" {
		return nil
	}
	out := []store.Entry{}
	for _, one := range held {
		if one.Cluster == cluster {
			continue
		}
		out = append(out, one)
	}
	return out
}

func (h *heldHistory) Reason() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.reason
}

func (h *heldHistory) recorded() []store.Entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]store.Entry{}, h.entries...)
}

func (h *heldHistory) only(t *testing.T) store.Entry {
	t.Helper()
	held := h.recorded()
	if len(held) != 1 {
		t.Fatalf("recorded %d entries, want exactly 1: %+v", len(held), held)
	}
	return held[0]
}

type writingBackend struct {
	notStubbed

	err      error
	action   api.ActionResult
	detail   api.ObjectDetail
	helmDone api.HelmActionResult
}

func (b *writingBackend) Action(_ context.Context, req actions.Request) (api.ActionResult, error) {
	result := b.action
	result.Action = string(req.Action)
	return result, b.err
}

func (b *writingBackend) ApplyObject(context.Context, api.ObjectRef, []byte) (api.ObjectDetail, error) {
	return b.detail, b.err
}

func (b *writingBackend) DeleteObject(context.Context, api.ObjectRef) error {
	return b.err
}

func (b *writingBackend) FluxAction(
	context.Context,
	api.ObjectRef,
	flux.Action,
) (api.FluxActionResult, error) {
	return api.FluxActionResult{}, b.err
}

func (b *writingBackend) ArgoAction(
	context.Context,
	api.ObjectRef,
	argocd.Request,
) (api.ArgoActionResult, error) {
	return api.ArgoActionResult{}, b.err
}

func (b *writingBackend) HelmRollback(
	context.Context,
	string,
	string,
	int64,
) (api.HelmActionResult, error) {
	return b.helmDone, b.err
}

func (b *writingBackend) HelmUninstall(context.Context, string, string) (api.HelmActionResult, error) {
	return b.helmDone, b.err
}

func (b *writingBackend) HelmUpgrade(
	context.Context,
	helm.UpgradeRequest,
) (api.HelmActionResult, error) {
	return b.helmDone, b.err
}

func (b *writingBackend) HelmInstall(
	context.Context,
	helm.InstallRequest,
) (api.HelmActionResult, error) {
	return b.helmDone, b.err
}

func recordingServer(t *testing.T, backend Backend) (*httptest.Server, *heldHistory) {
	t.Helper()
	held := &heldHistory{}
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	srv.UseHistory(held)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts, held
}

func pastServer(t *testing.T, held History) *httptest.Server {
	t.Helper()
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	if held != nil {
		srv.UseHistory(held)
	}
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func readBack(t *testing.T, body []byte) api.History {
	t.Helper()
	var got api.History
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
	return got
}

func TestHistoryIsReadableWithoutAStore(t *testing.T) {
	ts := pastServer(t, nil)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the view to load and say why it is empty: %s", resp.StatusCode, body)
	}
	got := readBack(t, body)
	if got.Reason != api.HistoryOff {
		t.Fatalf("reason = %q, want %q", got.Reason, api.HistoryOff)
	}
	if len(got.Entries) != 0 {
		t.Fatalf("entries = %v, want none", got.Entries)
	}
}

func TestHistoryIsServedEvenWithoutACluster(t *testing.T) {
	srv := New(&stubBackendCluster{backend: nil}, testAssets(), testToken)
	srv.UseHistory(&heldHistory{})
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want history readable when the cluster is gone: %s", resp.StatusCode, body)
	}
}

func TestHistoryComesBackNewestFirstAsItWasStored(t *testing.T) {
	held := &heldHistory{page: store.Page{
		Entries: []store.Entry{{
			ID:        7,
			At:        recordedAt,
			Verb:      "delete",
			Group:     "apps",
			Version:   "v1",
			Resource:  "deployments",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "web",
			Detail:    "gone",
			Outcome:   api.HistoryDone,
		}},
		More: true,
	}}
	ts := pastServer(t, held)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	got := readBack(t, body)
	if len(got.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(got.Entries))
	}
	entry := got.Entries[0]
	if entry.ID != 7 {
		t.Fatalf("id = %d, want the row's own id so a list can key on it", entry.ID)
	}
	if entry.At != "2026-08-29T09:30:00Z" {
		t.Fatalf("at = %q, want RFC3339 in UTC", entry.At)
	}
	if entry.Kind != "Deployment" || entry.Name != "web" {
		t.Fatalf("entry = %+v, want the object it names", entry)
	}
	if !got.More {
		t.Fatal("the page said it held everything when the held said otherwise")
	}
}

func TestHistoryIsScopedToTheConnectedCluster(t *testing.T) {
	held := &heldHistory{}
	ts := pastServer(t, held)

	doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	if held.asked().Cluster != stubClusterID {
		t.Fatalf("asked for cluster %q, want %q", held.asked().Cluster, stubClusterID)
	}
}

func TestALimitReachesTheStore(t *testing.T) {
	held := &heldHistory{}
	ts := pastServer(t, held)

	doRequest(t, http.MethodGet, ts.URL+"/api/history?limit=5", nil)

	if held.asked().Limit != 5 {
		t.Fatalf("asked for limit %d, want 5", held.asked().Limit)
	}
}

func TestHistoryPassesTheReasonItIsNotRecording(t *testing.T) {
	ts := pastServer(t, &heldHistory{reason: "the file is read-only"})

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	if readBack(t, body).Reason != "the file is read-only" {
		t.Fatalf("reason = %q, want the held's own words", readBack(t, body).Reason)
	}
}

func TestAnUnreadableHistoryIsReported(t *testing.T) {
	ts := pastServer(t, &heldHistory{readErr: errors.New("the database went away")})

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("status = %d, want a failure reported rather than an empty list: %s", resp.StatusCode, body)
	}
}

func TestALimitThatIsNotANumberIsRefused(t *testing.T) {
	ts := pastServer(t, &heldHistory{})

	for _, limit := range []string{"lots", "-1"} {
		resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history?limit="+limit, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for limit=%s = %d, want 400: %s", limit, resp.StatusCode, body)
		}
	}
}

func TestAnEmptyLimitIsLeftToTheStore(t *testing.T) {
	ts := pastServer(t, &heldHistory{})

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history?limit=", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the held's own default used: %s", resp.StatusCode, body)
	}
}

func TestHistoryCanBeCleared(t *testing.T) {
	held := &heldHistory{}
	ts := pastServer(t, held)

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/history", nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.StatusCode, body)
	}
	if held.forgotten != 1 {
		t.Fatalf("forgotten %d times, want once", held.forgotten)
	}
}

func TestClearingHistoryWithoutAStoreSaysSo(t *testing.T) {
	ts := pastServer(t, nil)

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/history", nil)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "not recording") {
		t.Fatalf("body = %s, want it to say history is off", body)
	}
}

func TestAFailureToClearIsReported(t *testing.T) {
	ts := pastServer(t, &heldHistory{forgetErr: errors.New("the database is read-only")})

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/history", nil)

	if resp.StatusCode == http.StatusNoContent {
		t.Fatal("clearing reported success when the held refused")
	}
}

func TestScalingIsRecordedWithWhatItScaledTo(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	resp, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=scale&replicas=3&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	entry := held.only(t)
	if entry.Verb != "scale" {
		t.Fatalf("verb = %q, want scale", entry.Verb)
	}
	if entry.Detail != "to 3 replicas" {
		t.Fatalf("detail = %q, want it to say what it scaled to", entry.Detail)
	}
	if entry.Name != "web" || entry.Namespace != "default" || entry.Resource != "deployments" {
		t.Fatalf("entry = %+v, want the object it acted on", entry)
	}
	if entry.Cluster != stubClusterID {
		t.Fatalf("cluster = %q, want %q", entry.Cluster, stubClusterID)
	}
	if !entry.At.Equal(recordedAt) {
		t.Fatalf("at = %s, want the instant the server was given", entry.At)
	}
	if entry.Outcome != api.HistoryDone {
		t.Fatalf("outcome = %q, want done", entry.Outcome)
	}
}

func TestScalingToOneSaysReplicaNotReplicas(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=scale&replicas=1&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if detail := held.only(t).Detail; detail != "to 1 replica" {
		t.Fatalf("detail = %q, want it to read as english", detail)
	}
}

func TestScalingToNoneSaysReplicas(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=scale&replicas=0&confirm=web&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if detail := held.only(t).Detail; detail != "to 0 replicas" {
		t.Fatalf("detail = %q, want plural for none", detail)
	}
}

func TestAnActionThatIsNotAScaleCarriesNoDetail(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if detail := held.only(t).Detail; detail != "" {
		t.Fatalf("detail = %q, want none for a restart", detail)
	}
}

func TestADryRunIsNotRecordedAsAChange(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=scale&replicas=0&dryRun=true&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if held := held.recorded(); len(held) != 0 {
		t.Fatalf("recorded %+v, want nothing; a dry run changed nothing", held)
	}
}

func TestARefusedWriteIsRecordedAsRefused(t *testing.T) {
	denied := apierrors.NewForbidden(schema.GroupResource{Resource: "deployments"}, "web", errors.New("nope"))
	ts, held := recordingServer(t, &writingBackend{err: denied})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	entry := held.only(t)
	if entry.Outcome != api.HistoryRefused {
		t.Fatalf("outcome = %q, want refused", entry.Outcome)
	}
	if entry.Message == "" {
		t.Fatal("a refused write recorded no reason")
	}
}

func TestAWriteThatBrokeIsRecordedAsFailed(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{err: api.ErrInternal})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if outcome := held.only(t).Outcome; outcome != api.HistoryFailed {
		t.Fatalf("outcome = %q, want failed", outcome)
	}
}

func TestAPartialDrainIsRecordedAsFailed(t *testing.T) {
	result := api.ActionResult{
		Message: "1 pod failed to evict",
		Pods: []api.PodOutcome{{
			Namespace: "default",
			Name:      "web",
			Outcome:   api.OutcomeFailed,
			Reason:    "the admission webhook failed",
		}},
	}
	ts, held := recordingServer(t, &writingBackend{action: result})

	resp, _ := doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=drain&version=v1&resource=nodes&name=worker-1", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the useful drain result returned", resp.StatusCode)
	}
	entry := held.only(t)
	if entry.Outcome != api.HistoryFailed {
		t.Fatalf("outcome = %q, want the partial drain recorded as failed", entry.Outcome)
	}
	if !strings.Contains(entry.Message, result.Message) {
		t.Fatalf("message = %q, want it to include %q", entry.Message, result.Message)
	}
}

func TestApplyingAnObjectIsRecordedWithItsKind(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{detail: api.ObjectDetail{Kind: "Deployment"}})

	doRequest(t, http.MethodPut,
		ts.URL+"/api/object?group=apps&version=v1&resource=deployments&namespace=default&name=web",
		strings.NewReader("kind: Deployment"))

	entry := held.only(t)
	if entry.Verb != verbApply {
		t.Fatalf("verb = %q, want apply", entry.Verb)
	}
	if entry.Kind != "Deployment" {
		t.Fatalf("kind = %q, want the kind the apply came back with", entry.Kind)
	}
}

func TestDeletingAnObjectIsRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodDelete,
		ts.URL+"/api/object?group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if verb := held.only(t).Verb; verb != verbDelete {
		t.Fatalf("verb = %q, want delete", verb)
	}
}

func TestAFluxActionIsRecordedByItsOwnName(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/flux/action?action=suspend&group=kustomize.toolkit.fluxcd.io&version=v1&resource=kustomizations&namespace=flux-system&name=apps", nil)

	if verb := held.only(t).Verb; verb != "suspend" {
		t.Fatalf("verb = %q, want suspend", verb)
	}
}

func TestAnArgoSyncIsRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=sync&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web",
		strings.NewReader(`{"prune":true}`))

	entry := held.only(t)
	if entry.Verb != "sync" {
		t.Fatalf("verb = %q, want sync", entry.Verb)
	}
	if entry.Detail != "with prune" {
		t.Fatalf("detail = %q, want the prune flag noted", entry.Detail)
	}
}

func TestAnArgoRollbackRecordsTheRevision(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=rollback&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web",
		strings.NewReader(`{"revision":4}`))

	if detail := held.only(t).Detail; detail != "to revision 4" {
		t.Fatalf("detail = %q, want the revision it went back to", detail)
	}
}

func TestAnArgoSyncOfMarkedResourcesSaysHowMany(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=sync&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web",
		strings.NewReader(`{"resources":[{"kind":"Deployment","name":"web"},{"kind":"Service","name":"web"}]}`))

	if detail := held.only(t).Detail; detail != "2 selected resources" {
		t.Fatalf("detail = %q, want the count of what was marked", detail)
	}
}

func TestAPlainArgoRefreshCarriesNoDetail(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=refresh&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web", nil)

	if detail := held.only(t).Detail; detail != "" {
		t.Fatalf("detail = %q, want none", detail)
	}
}

func TestAnArgoDryRunIsNotRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=sync&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web",
		strings.NewReader(`{"dryRun":true}`))

	if held := held.recorded(); len(held) != 0 {
		t.Fatalf("recorded %+v, want nothing for a dry run", held)
	}
}

func TestUninstallingAReleaseIsRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/action?action=uninstall&namespace=default&name=web", nil)

	entry := held.only(t)
	if entry.Verb != verbUninstall {
		t.Fatalf("verb = %q, want uninstall", entry.Verb)
	}
	if entry.Kind != kindRelease {
		t.Fatalf("kind = %q, want Release", entry.Kind)
	}
}

func TestRollingBackAReleaseRecordsTheRevision(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/helm/action?action=rollback&revision=2&namespace=default&name=web", nil)

	entry := held.only(t)
	if entry.Verb != verbRollback {
		t.Fatalf("verb = %q, want rollback", entry.Verb)
	}
	if entry.Detail != "to revision 2" {
		t.Fatalf("detail = %q, want the revision", entry.Detail)
	}
}

func TestInstallingAChartIsRecordedWithWhatWasInstalled(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/install",
		strings.NewReader(`{"namespace":"default","name":"web","chart":"nginx","repo":"https://charts","version":"1.2.3"}`))

	entry := held.only(t)
	if entry.Verb != verbInstall {
		t.Fatalf("verb = %q, want install", entry.Verb)
	}
	if entry.Detail != "nginx 1.2.3" {
		t.Fatalf("detail = %q, want the chart and version", entry.Detail)
	}
}

func TestUpgradingAChartIsRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/upgrade",
		strings.NewReader(`{"namespace":"default","name":"web","chart":"nginx","repo":"https://charts","version":"2.0.0"}`))

	if verb := held.only(t).Verb; verb != verbUpgrade {
		t.Fatalf("verb = %q, want upgrade", verb)
	}
}

func TestAHelmDryRunIsNotRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/upgrade?dryRun=true",
		strings.NewReader(`{"namespace":"default","name":"web","chart":"nginx","repo":"https://charts","version":"2.0.0"}`))

	if held := held.recorded(); len(held) != 0 {
		t.Fatalf("recorded %+v, want nothing for a dry run", held)
	}
}

func TestAFailedHelmDryRunIsStillNotRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{err: errors.New("the chart is broken")})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/upgrade?dryRun=true",
		strings.NewReader(`{"namespace":"default","name":"web","chart":"nginx","repo":"https://charts","version":"2.0.0"}`))

	if held := held.recorded(); len(held) != 0 {
		t.Fatalf("recorded %+v; a dry run that failed still changed nothing", held)
	}
}

func TestAWriteStillSucceedsWhenItCannotBeRecorded(t *testing.T) {
	held := &heldHistory{recordErr: errors.New("the database is gone")}
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	srv.UseHistory(held)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the action to stand even though recording failed: %s", resp.StatusCode, body)
	}
}

func TestNothingIsRecordedWithoutAStore(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	resp, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the action to work with history switched off: %s", resp.StatusCode, body)
	}
}

func TestALimitIsPassedThroughToTheStore(t *testing.T) {
	held := &heldHistory{}
	ts := pastServer(t, held)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/history?limit=5", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
}

func (b *writingBackend) StartDebug(
	context.Context,
	debugcontainer.Request,
) (api.DebugSession, error) {
	return api.DebugSession{}, b.err
}

func (b *writingBackend) StartNodeShell(context.Context, string) (api.NodeShellSession, error) {
	return api.NodeShellSession{}, b.err
}

func (b *writingBackend) RemoveNodeShell(context.Context, string) {}

func (b *writingBackend) StartExec(
	context.Context,
	exec.Request,
	io.Writer,
) (*exec.Session, error) {
	return nil, b.err
}

func TestAttachingADebugContainerIsRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=default&pod=web&profile=general", nil)

	entry := held.only(t)
	if entry.Verb != verbDebug {
		t.Fatalf("verb = %q, want debug", entry.Verb)
	}
	if entry.Kind != kindPod || entry.Resource != "pods" || entry.Name != "web" {
		t.Fatalf("entry = %+v, want the pod it attached to", entry)
	}
	if entry.Detail != "with the general profile" {
		t.Fatalf("detail = %q, want the profile named", entry.Detail)
	}
}

func TestADebugContainerWithNoProfileNamesTheContainer(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=default&pod=web&container=app", nil)

	if detail := held.only(t).Detail; detail != "into app" {
		t.Fatalf("detail = %q, want the container named", detail)
	}
}

func TestADebugContainerWithNothingToSayCarriesNoDetail(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=default&pod=web", nil)

	if detail := held.only(t).Detail; detail != "" {
		t.Fatalf("detail = %q, want none", detail)
	}
}

func TestADebugContainerThatWasRefusedIsStillRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{err: errors.New("ephemeral containers are disabled")})

	doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=default&pod=web", nil)

	if outcome := held.only(t).Outcome; outcome == api.HistoryDone {
		t.Fatal("a debug container that never started was recorded as done")
	}
}

func TestARootShellOnANodeIsRecorded(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{err: errors.New("this cluster refuses privileged pods")})

	conn, _, err := websocket.Dial(context.Background(),
		"ws"+strings.TrimPrefix(ts.URL, "http")+"/api/nodeshell?node=worker-1", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	waitFor(t, func() bool { return len(held.recorded()) == 1 })

	entry := held.only(t)
	if entry.Verb != verbNodeShell {
		t.Fatalf("verb = %q, want a node shell recorded; it creates a privileged pod", entry.Verb)
	}
	if entry.Kind != kindNode || entry.Resource != "nodes" || entry.Name != "worker-1" {
		t.Fatalf("entry = %+v, want the node it was opened on", entry)
	}
	if entry.Namespace != "" {
		t.Fatalf("namespace = %q, want none for a node", entry.Namespace)
	}
}

func TestAShellIntoAPodIsRecordedEvenWhenItWillNotOpen(t *testing.T) {
	ts, held := recordingServer(t, &writingBackend{err: errors.New("the container is not running")})

	conn, _, err := websocket.Dial(context.Background(),
		"ws"+strings.TrimPrefix(ts.URL, "http")+"/api/exec?namespace=default&pod=web&container=app", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	waitFor(t, func() bool { return len(held.recorded()) == 1 })

	entry := held.only(t)
	if entry.Verb != verbExec {
		t.Fatalf("verb = %q, want exec", entry.Verb)
	}
	if entry.Detail != "into app" {
		t.Fatalf("detail = %q, want the container named", entry.Detail)
	}
	if entry.Outcome == api.HistoryDone {
		t.Fatal("a shell that never opened was recorded as done")
	}
}

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	for range 200 {
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the recording never arrived")
}

func TestAWriteIsRecordedEvenIfTheBrowserWalkedAway(t *testing.T) {
	held := &heldHistory{}
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	srv.UseHistory(held)
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/action", http.NoBody).WithContext(gone)

	srv.record(req, change{verb: "restart", ref: api.ObjectRef{Resource: "deployments", Name: "web"}})

	if len(held.recorded()) != 1 {
		t.Fatal("the cluster changed and nothing recorded it because the client hung up")
	}
	if held.only(t).Actor != "local" {
		t.Fatalf("actor = %q, want local for a request without a signed-in identity", held.only(t).Actor)
	}
}

func TestAnActionRecordsWhoMadeIt(t *testing.T) {
	tests := []struct {
		name     string
		identity *auth.Identity
		want     string
	}{
		{name: "local mode", want: "local"},
		{name: "authentication disabled", identity: &auth.Identity{Role: auth.RoleAdmin}, want: "anonymous"},
		{name: "signed in", identity: &auth.Identity{User: "alice@example.com", Role: auth.RoleEditor}, want: "alice@example.com"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/action", http.NoBody)
			if test.identity != nil {
				req = req.WithContext(auth.WithIdentity(req.Context(), *test.identity))
			}

			if got := actorOf(req); got != test.want {
				t.Fatalf("actor = %q, want %q", got, test.want)
			}
		})
	}
}

func realStoreServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	held, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "spinoza", "history.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = held.Close() })
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	srv.UseHistory(held)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts, held
}

func TestAChangeMadeThroughTheApiComesBackFromTheRealStore(t *testing.T) {
	ts, _ := realStoreServer(t)

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=scale&replicas=3&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)
	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	got := readBack(t, body)
	if len(got.Entries) != 1 {
		t.Fatalf("entries = %d, want the scale that just happened: %s", len(got.Entries), body)
	}
	entry := got.Entries[0]
	if entry.ID == 0 {
		t.Fatal("the row came back without the id the database gave it")
	}
	if entry.At != "2026-08-29T09:30:00Z" {
		t.Fatalf("at = %q, want the instant the server was given", entry.At)
	}
	if entry.Verb != "scale" || entry.Detail != "to 3 replicas" {
		t.Fatalf("entry = %+v, want the scale it recorded", entry)
	}
	if entry.Actor != "local" {
		t.Fatalf("actor = %q, want local for the local-token request", entry.Actor)
	}
	if entry.Resource != "deployments" || entry.Namespace != "default" || entry.Name != "web" {
		t.Fatalf("entry = %+v, want the object it acted on", entry)
	}
	if entry.Outcome != api.HistoryDone {
		t.Fatalf("outcome = %q, want done", entry.Outcome)
	}
	if got.Reason != "" {
		t.Fatalf("reason = %q, want none from a working held", got.Reason)
	}
}

func TestClearingThroughTheApiEmptiesTheRealStore(t *testing.T) {
	ts, _ := realStoreServer(t)
	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/history", nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)
	if len(readBack(t, body).Entries) != 0 {
		t.Fatalf("history survived being cleared: %s", body)
	}
}

func TestAnotherClustersHistoryIsNotShown(t *testing.T) {
	ts, held := realStoreServer(t)
	elsewhere := store.Entry{
		Cluster: "https://p-mk2:6443",
		At:      recordedAt,
		Verb:    "delete",
		Name:    "somewhere-else",
		Outcome: api.HistoryDone,
	}
	if err := held.For(elsewhere.Cluster).Record(t.Context(), elsewhere); err != nil {
		t.Fatalf("record: %v", err)
	}
	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	for _, entry := range readBack(t, body).Entries {
		if entry.Name == "somewhere-else" {
			t.Fatalf("a row from another cluster leaked into this one's history: %s", body)
		}
	}
}

func TestAPageFromTheRealStoreSaysWhenItLeftSomethingOut(t *testing.T) {
	ts, _ := realStoreServer(t)
	for range 3 {
		doRequest(t, http.MethodPost,
			ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)
	}

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history?limit=2", nil)

	got := readBack(t, body)
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want the 2 asked for", len(got.Entries))
	}
	if !got.More {
		t.Fatal("the page dropped a row and did not say so")
	}
}

func TestEveryStoredHistoryFieldReachesTheWire(t *testing.T) {
	carriedOnTheRequest := map[string]string{}

	stored := reflect.TypeFor[store.Entry]()
	sent := reflect.TypeFor[api.HistoryEntry]()
	onTheWire := map[string]bool{}
	for field := range sent.Fields() {
		onTheWire[field.Name] = true
	}

	for field := range stored.Fields() {
		name := field.Name
		if onTheWire[name] {
			continue
		}
		why, deliberate := carriedOnTheRequest[name]
		if !deliberate {
			t.Errorf(
				"store.Entry.%s never reaches api.HistoryEntry; carry it in entriesOf, "+
					"or say here why it stays behind", name,
			)
			continue
		}
		t.Logf("store.Entry.%s stays behind: %s", name, why)
	}
}

func TestTheWireCarriesNoHistoryFieldTheStoreCannotFill(t *testing.T) {
	saidByTheReader := map[string]string{
		"Source": "which of the two tables the row came from, which is not a column in either",
		"Was":    "what a change moved from, which only the changes table records",
	}

	stored := reflect.TypeFor[store.Entry]()
	held := map[string]bool{}
	for field := range stored.Fields() {
		held[field.Name] = true
	}

	sent := reflect.TypeFor[api.HistoryEntry]()
	for field := range sent.Fields() {
		name := field.Name
		if held[name] {
			continue
		}
		why, deliberate := saidByTheReader[name]
		if !deliberate {
			t.Errorf("api.HistoryEntry.%s has no field in store.Entry to fill it", name)
			continue
		}
		t.Logf("api.HistoryEntry.%s is filled by the reader: %s", name, why)
	}
}

func TestEntriesOfCarriesEveryValue(t *testing.T) {
	at := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	one := store.Entry{
		ID: 7, Cluster: "p-mk2", At: at, Verb: "scale",
		Actor: "alice@example.com",
		Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment",
		Namespace: "audit-probe", Name: "target", Detail: "to 2 replicas",
		Outcome: "done", Message: "all good",
	}

	got := entriesOf([]store.Entry{one})

	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	want := api.HistoryEntry{
		ID: 7, Source: api.HistoryAction, Cluster: "p-mk2", At: "2026-08-29T12:00:00Z", Verb: "scale",
		Actor: "alice@example.com",
		Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment",
		Namespace: "audit-probe", Name: "target", Detail: "to 2 replicas",
		Outcome: "done", Message: "all good",
	}
	if got[0] != want {
		t.Fatalf("entry = %+v, want %+v", got[0], want)
	}
}
