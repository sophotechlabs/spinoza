package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/history"
)

var recordedAt = time.Date(2026, 8, 29, 9, 30, 0, 0, time.UTC)

const stubClusterID = "https://p-mk1:6443"

type heldHistory struct {
	mu        sync.Mutex
	entries   []history.Entry
	page      history.Page
	reason    string
	lastQuery history.Query
	recordErr error
	readErr   error
	forgetErr error
	forgotten int

	forgotCluster string
}

func (h *heldHistory) For(cluster string) history.Recorder {
	return heldWriter{into: h, cluster: cluster}
}

type heldWriter struct {
	into    *heldHistory
	cluster string
}

func (w heldWriter) Record(_ context.Context, entry history.Entry) error {
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

func (h *heldHistory) Recent(_ context.Context, query history.Query) (history.Page, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastQuery = query
	if h.readErr != nil {
		return history.Page{}, h.readErr
	}
	return h.page, nil
}

func (h *heldHistory) asked() history.Query {
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

func kept(held []history.Entry, cluster string) []history.Entry {
	if cluster == "" {
		return nil
	}
	out := []history.Entry{}
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

func (h *heldHistory) recorded() []history.Entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]history.Entry{}, h.entries...)
}

func (h *heldHistory) only(t *testing.T) history.Entry {
	t.Helper()
	held := h.recorded()
	if len(held) != 1 {
		t.Fatalf("recorded %d entries, want exactly 1: %+v", len(held), held)
	}
	return held[0]
}

type writingBackend struct {
	Backend

	err      error
	detail   api.ObjectDetail
	helmDone api.HelmActionResult
}

func (b *writingBackend) Action(_ context.Context, req actions.Request) (api.ActionResult, error) {
	return api.ActionResult{Action: string(req.Action)}, b.err
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
	store := &heldHistory{}
	srv := New(&stubBackendCluster{backend: backend}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	srv.UseHistory(store)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts, store
}

func pastServer(t *testing.T, store History) *httptest.Server {
	t.Helper()
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	if store != nil {
		srv.UseHistory(store)
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
	store := &heldHistory{page: history.Page{
		Entries: []history.Entry{{
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
	ts := pastServer(t, store)

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
		t.Fatal("the page said it held everything when the store said otherwise")
	}
}

func TestHistoryIsScopedToTheConnectedCluster(t *testing.T) {
	store := &heldHistory{}
	ts := pastServer(t, store)

	doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	if store.asked().Cluster != stubClusterID {
		t.Fatalf("asked for cluster %q, want %q", store.asked().Cluster, stubClusterID)
	}
}

func TestALimitReachesTheStore(t *testing.T) {
	store := &heldHistory{}
	ts := pastServer(t, store)

	doRequest(t, http.MethodGet, ts.URL+"/api/history?limit=5", nil)

	if store.asked().Limit != 5 {
		t.Fatalf("asked for limit %d, want 5", store.asked().Limit)
	}
}

func TestHistoryPassesTheReasonItIsNotRecording(t *testing.T) {
	ts := pastServer(t, &heldHistory{reason: "the file is read-only"})

	_, body := doRequest(t, http.MethodGet, ts.URL+"/api/history", nil)

	if readBack(t, body).Reason != "the file is read-only" {
		t.Fatalf("reason = %q, want the store's own words", readBack(t, body).Reason)
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
		t.Fatalf("status = %d, want the store's own default used: %s", resp.StatusCode, body)
	}
}

func TestHistoryCanBeCleared(t *testing.T) {
	store := &heldHistory{}
	ts := pastServer(t, store)

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/history", nil)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", resp.StatusCode, body)
	}
	if store.forgotten != 1 {
		t.Fatalf("forgotten %d times, want once", store.forgotten)
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
		t.Fatal("clearing reported success when the store refused")
	}
}

func TestScalingIsRecordedWithWhatItScaledTo(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	resp, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=scale&replicas=3&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	entry := store.only(t)
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
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=scale&replicas=1&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if detail := store.only(t).Detail; detail != "to 1 replica" {
		t.Fatalf("detail = %q, want it to read as english", detail)
	}
}

func TestScalingToNoneSaysReplicas(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=scale&replicas=0&confirm=web&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if detail := store.only(t).Detail; detail != "to 0 replicas" {
		t.Fatalf("detail = %q, want plural for none", detail)
	}
}

func TestAnActionThatIsNotAScaleCarriesNoDetail(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if detail := store.only(t).Detail; detail != "" {
		t.Fatalf("detail = %q, want none for a restart", detail)
	}
}

func TestADryRunIsNotRecordedAsAChange(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=scale&replicas=0&dryRun=true&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if held := store.recorded(); len(held) != 0 {
		t.Fatalf("recorded %+v, want nothing; a dry run changed nothing", held)
	}
}

func TestARefusedWriteIsRecordedAsRefused(t *testing.T) {
	denied := apierrors.NewForbidden(schema.GroupResource{Resource: "deployments"}, "web", errors.New("nope"))
	ts, store := recordingServer(t, &writingBackend{err: denied})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	entry := store.only(t)
	if entry.Outcome != api.HistoryRefused {
		t.Fatalf("outcome = %q, want refused", entry.Outcome)
	}
	if entry.Message == "" {
		t.Fatal("a refused write recorded no reason")
	}
}

func TestAWriteThatBrokeIsRecordedAsFailed(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{err: api.ErrInternal})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/action?action=restart&group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if outcome := store.only(t).Outcome; outcome != api.HistoryFailed {
		t.Fatalf("outcome = %q, want failed", outcome)
	}
}

func TestApplyingAnObjectIsRecordedWithItsKind(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{detail: api.ObjectDetail{Kind: "Deployment"}})

	doRequest(t, http.MethodPut,
		ts.URL+"/api/object?group=apps&version=v1&resource=deployments&namespace=default&name=web",
		strings.NewReader("kind: Deployment"))

	entry := store.only(t)
	if entry.Verb != verbApply {
		t.Fatalf("verb = %q, want apply", entry.Verb)
	}
	if entry.Kind != "Deployment" {
		t.Fatalf("kind = %q, want the kind the apply came back with", entry.Kind)
	}
}

func TestDeletingAnObjectIsRecorded(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodDelete,
		ts.URL+"/api/object?group=apps&version=v1&resource=deployments&namespace=default&name=web", nil)

	if verb := store.only(t).Verb; verb != verbDelete {
		t.Fatalf("verb = %q, want delete", verb)
	}
}

func TestAFluxActionIsRecordedByItsOwnName(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/flux/action?action=suspend&group=kustomize.toolkit.fluxcd.io&version=v1&resource=kustomizations&namespace=flux-system&name=apps", nil)

	if verb := store.only(t).Verb; verb != "suspend" {
		t.Fatalf("verb = %q, want suspend", verb)
	}
}

func TestAnArgoSyncIsRecorded(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=sync&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web",
		strings.NewReader(`{"prune":true}`))

	entry := store.only(t)
	if entry.Verb != "sync" {
		t.Fatalf("verb = %q, want sync", entry.Verb)
	}
	if entry.Detail != "with prune" {
		t.Fatalf("detail = %q, want the prune flag noted", entry.Detail)
	}
}

func TestAnArgoRollbackRecordsTheRevision(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=rollback&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web",
		strings.NewReader(`{"revision":4}`))

	if detail := store.only(t).Detail; detail != "to revision 4" {
		t.Fatalf("detail = %q, want the revision it went back to", detail)
	}
}

func TestAnArgoSyncOfMarkedResourcesSaysHowMany(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=sync&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web",
		strings.NewReader(`{"resources":[{"kind":"Deployment","name":"web"},{"kind":"Service","name":"web"}]}`))

	if detail := store.only(t).Detail; detail != "2 selected resources" {
		t.Fatalf("detail = %q, want the count of what was marked", detail)
	}
}

func TestAPlainArgoRefreshCarriesNoDetail(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=refresh&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web", nil)

	if detail := store.only(t).Detail; detail != "" {
		t.Fatalf("detail = %q, want none", detail)
	}
}

func TestAnArgoDryRunIsNotRecorded(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/argocd/action?action=sync&group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=web",
		strings.NewReader(`{"dryRun":true}`))

	if held := store.recorded(); len(held) != 0 {
		t.Fatalf("recorded %+v, want nothing for a dry run", held)
	}
}

func TestUninstallingAReleaseIsRecorded(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/action?action=uninstall&namespace=default&name=web", nil)

	entry := store.only(t)
	if entry.Verb != verbUninstall {
		t.Fatalf("verb = %q, want uninstall", entry.Verb)
	}
	if entry.Kind != kindRelease {
		t.Fatalf("kind = %q, want Release", entry.Kind)
	}
}

func TestRollingBackAReleaseRecordsTheRevision(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost,
		ts.URL+"/api/helm/action?action=rollback&revision=2&namespace=default&name=web", nil)

	entry := store.only(t)
	if entry.Verb != verbRollback {
		t.Fatalf("verb = %q, want rollback", entry.Verb)
	}
	if entry.Detail != "to revision 2" {
		t.Fatalf("detail = %q, want the revision", entry.Detail)
	}
}

func TestInstallingAChartIsRecordedWithWhatWasInstalled(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/install",
		strings.NewReader(`{"namespace":"default","name":"web","chart":"nginx","repo":"https://charts","version":"1.2.3"}`))

	entry := store.only(t)
	if entry.Verb != verbInstall {
		t.Fatalf("verb = %q, want install", entry.Verb)
	}
	if entry.Detail != "nginx 1.2.3" {
		t.Fatalf("detail = %q, want the chart and version", entry.Detail)
	}
}

func TestUpgradingAChartIsRecorded(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/upgrade",
		strings.NewReader(`{"namespace":"default","name":"web","chart":"nginx","repo":"https://charts","version":"2.0.0"}`))

	if verb := store.only(t).Verb; verb != verbUpgrade {
		t.Fatalf("verb = %q, want upgrade", verb)
	}
}

func TestAHelmDryRunIsNotRecorded(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/upgrade?dryRun=true",
		strings.NewReader(`{"namespace":"default","name":"web","chart":"nginx","repo":"https://charts","version":"2.0.0"}`))

	if held := store.recorded(); len(held) != 0 {
		t.Fatalf("recorded %+v, want nothing for a dry run", held)
	}
}

func TestAFailedHelmDryRunIsStillNotRecorded(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{err: errors.New("the chart is broken")})

	doRequest(t, http.MethodPost, ts.URL+"/api/helm/upgrade?dryRun=true",
		strings.NewReader(`{"namespace":"default","name":"web","chart":"nginx","repo":"https://charts","version":"2.0.0"}`))

	if held := store.recorded(); len(held) != 0 {
		t.Fatalf("recorded %+v; a dry run that failed still changed nothing", held)
	}
}

func TestAWriteStillSucceedsWhenItCannotBeRecorded(t *testing.T) {
	store := &heldHistory{recordErr: errors.New("the database is gone")}
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	srv.UseHistory(store)
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
	store := &heldHistory{}
	ts := pastServer(t, store)

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
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=default&pod=web&profile=general", nil)

	entry := store.only(t)
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
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=default&pod=web&container=app", nil)

	if detail := store.only(t).Detail; detail != "into app" {
		t.Fatalf("detail = %q, want the container named", detail)
	}
}

func TestADebugContainerWithNothingToSayCarriesNoDetail(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{})

	doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=default&pod=web", nil)

	if detail := store.only(t).Detail; detail != "" {
		t.Fatalf("detail = %q, want none", detail)
	}
}

func TestADebugContainerThatWasRefusedIsStillRecorded(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{err: errors.New("ephemeral containers are disabled")})

	doRequest(t, http.MethodPost, ts.URL+"/api/debug?namespace=default&pod=web", nil)

	if outcome := store.only(t).Outcome; outcome == api.HistoryDone {
		t.Fatal("a debug container that never started was recorded as done")
	}
}

func TestARootShellOnANodeIsRecorded(t *testing.T) {
	ts, store := recordingServer(t, &writingBackend{err: errors.New("this cluster refuses privileged pods")})

	conn, _, err := websocket.Dial(context.Background(),
		"ws"+strings.TrimPrefix(ts.URL, "http")+"/api/nodeshell?node=worker-1", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	waitFor(t, func() bool { return len(store.recorded()) == 1 })

	entry := store.only(t)
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
	ts, store := recordingServer(t, &writingBackend{err: errors.New("the container is not running")})

	conn, _, err := websocket.Dial(context.Background(),
		"ws"+strings.TrimPrefix(ts.URL, "http")+"/api/exec?namespace=default&pod=web&container=app", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	waitFor(t, func() bool { return len(store.recorded()) == 1 })

	entry := store.only(t)
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
	store := &heldHistory{}
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	srv.UseHistory(store)
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/action", http.NoBody).WithContext(gone)

	srv.record(req, change{verb: "restart", ref: api.ObjectRef{Resource: "deployments", Name: "web"}})

	if len(store.recorded()) != 1 {
		t.Fatal("the cluster changed and nothing recorded it because the client hung up")
	}
}

func realStoreServer(t *testing.T) (*httptest.Server, *history.Store) {
	t.Helper()
	store, err := history.Open(t.Context(), filepath.Join(t.TempDir(), "spinoza", "history.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := New(&stubBackendCluster{backend: &writingBackend{}}, testAssets(), testToken)
	srv.now = func() time.Time { return recordedAt }
	srv.UseHistory(store)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts, store
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
	if entry.Resource != "deployments" || entry.Namespace != "default" || entry.Name != "web" {
		t.Fatalf("entry = %+v, want the object it acted on", entry)
	}
	if entry.Outcome != api.HistoryDone {
		t.Fatalf("outcome = %q, want done", entry.Outcome)
	}
	if got.Reason != "" {
		t.Fatalf("reason = %q, want none from a working store", got.Reason)
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
	ts, store := realStoreServer(t)
	elsewhere := history.Entry{
		Cluster: "https://p-mk2:6443",
		At:      recordedAt,
		Verb:    "delete",
		Name:    "somewhere-else",
		Outcome: api.HistoryDone,
	}
	if err := store.For(elsewhere.Cluster).Record(t.Context(), elsewhere); err != nil {
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
