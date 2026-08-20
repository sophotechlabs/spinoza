package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type stubCluster struct {
	mu          sync.Mutex
	mgr         *resources.Manager
	elsewhere   map[string]string
	over        map[string][]*unstructured.Unstructured
	listErr     error
	readErr     error
	kubeconfigs []api.Kubeconfig
	current     api.ContextRef
	useErr      error
	changeErr   error
	protectErr  error
	protection  string
	switched    []api.ContextRef
	added       []string
	removed     []string
}

func fixed(mgr *resources.Manager) *stubCluster {
	return &stubCluster{
		mgr: mgr,
		kubeconfigs: []api.Kubeconfig{{
			Label: "/home/arch/.kube/config",
			Contexts: []api.KubeContext{
				{Name: "p-mk1", Cluster: "p-mk1"},
				{Name: "p-mk2", Cluster: "p-mk2"},
			},
		}},
		current: api.ContextRef{Name: "p-mk2"},
	}
}

func (s *stubCluster) Manager() Backend {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mgr
}

func (s *stubCluster) Contexts() api.ContextList {
	s.mu.Lock()
	defer s.mu.Unlock()
	return api.ContextList{Current: s.current, Kubeconfigs: s.kubeconfigs, Protection: s.protection}
}

func (s *stubCluster) Use(ref api.ContextRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.useErr != nil {
		return s.useErr
	}
	s.switched = append(s.switched, ref)
	s.current = ref
	return nil
}

func (s *stubCluster) AddKubeconfig(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.changeErr != nil {
		return s.changeErr
	}
	s.added = append(s.added, path)
	s.kubeconfigs = append(s.kubeconfigs, api.Kubeconfig{Label: path, Path: path, Removable: true})
	return nil
}

func (s *stubCluster) RemoveKubeconfig(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.changeErr != nil {
		return s.changeErr
	}
	s.removed = append(s.removed, path)
	return nil
}

func (s *stubCluster) Protect(protected bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.protectErr != nil {
		return s.protectErr
	}
	s.protection = api.ProtectionOpen
	if protected {
		s.protection = api.ProtectionProtected
	}
	return nil
}

func (s *stubCluster) List(
	_ context.Context,
	ref api.ContextRef,
	target api.ObjectRef,
) ([]*unstructured.Unstructured, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.over[ref.Name+"/"+target.Namespace], nil
}

func (s *stubCluster) Read(_ context.Context, ref api.ContextRef, target api.ObjectRef) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr != nil {
		return "", s.readErr
	}
	found, held := s.elsewhere[ref.Name+"/"+target.Namespace+"/"+target.Name]
	if !held {
		return "", apierrors.NewNotFound(schema.GroupResource{Resource: target.Resource}, target.Name)
	}
	return found, nil
}

func (s *stubCluster) Protected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.protection == api.ProtectionProtected
}

func (s *stubCluster) calls() []api.ContextRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]api.ContextRef{}, s.switched...)
}

func (s *stubCluster) addCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.added...)
}

func (s *stubCluster) removeCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.removed...)
}

func contextServer(t *testing.T, cluster Cluster) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(authed(New(cluster, testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func TestContextsAreListedWithTheCurrentOne(t *testing.T) {
	mgr, _ := testManager(t)
	ts := contextServer(t, fixed(mgr))

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/contexts", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	list := decodeContexts(t, body)
	if len(list.Kubeconfigs) != 1 || len(list.Kubeconfigs[0].Contexts) != 2 {
		t.Fatalf("kubeconfigs = %v", list.Kubeconfigs)
	}
	if list.Current.Name != "p-mk2" {
		t.Fatalf("current = %q", list.Current.Name)
	}
}

func TestSwitchingContextsAsksTheCluster(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/contexts?name=p-mk1", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(cluster.calls()) != 1 || cluster.calls()[0].Name != "p-mk1" {
		t.Fatalf("switched = %v", cluster.calls())
	}
	list := decodeContexts(t, body)
	if list.Current.Name != "p-mk1" {
		t.Fatalf("current = %q, want the new context echoed back", list.Current.Name)
	}
}

func TestSwitchingCarriesTheKubeconfigTheContextCameFrom(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/contexts?name=beta&kubeconfig=%2Ftmp%2Fother.yaml", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	want := api.ContextRef{Kubeconfig: "/tmp/other.yaml", Name: "beta"}
	if len(cluster.calls()) != 1 || cluster.calls()[0] != want {
		t.Fatalf("switched = %v, want %v; two kubeconfigs may hold the same context name", cluster.calls(), want)
	}
}

func TestSwitchingRequiresAName(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/contexts", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(cluster.calls()) != 0 {
		t.Fatalf("switched = %v on a bad request", cluster.calls())
	}
}

func TestAFailedSwitchKeepsTheOldContext(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	cluster.useErr = errors.New("context \"gone\" does not exist")
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/contexts?name=gone", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a failed switch reported success")
	}
	if !strings.Contains(string(body), "does not exist") {
		t.Fatalf("body = %s, want the reason", body)
	}
	if cluster.Contexts().Current.Name != "p-mk2" {
		t.Fatalf("current = %q, want the old context kept", cluster.Contexts().Current.Name)
	}
}

func TestContextsRejectsADelete(t *testing.T) {
	mgr, _ := testManager(t)
	ts := contextServer(t, fixed(mgr))

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/contexts", nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestAddingAKubeconfigListsItBack(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/kubeconfigs?path=%2Ftmp%2Fother.yaml", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(cluster.addCalls()) != 1 || cluster.addCalls()[0] != "/tmp/other.yaml" {
		t.Fatalf("added = %v", cluster.addCalls())
	}
	list := decodeContexts(t, body)
	if len(list.Kubeconfigs) != 2 {
		t.Fatalf("kubeconfigs = %v, want the added one in the answer", list.Kubeconfigs)
	}
}

func TestAddingAKubeconfigNeedsAPath(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/kubeconfigs", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(cluster.addCalls()) != 0 {
		t.Fatalf("added = %v on a bad request", cluster.addCalls())
	}
}

func TestAKubeconfigTheClusterRefusesIsReported(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	cluster.changeErr = errors.New("that file is not a kubeconfig")
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/kubeconfigs?path=%2Ftmp%2Fnotes.txt", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a file that is not a kubeconfig was accepted")
	}
	if !strings.Contains(string(body), "not a kubeconfig") {
		t.Fatalf("body = %s, want the reason", body)
	}
}

func TestRemovingAKubeconfigReachesTheCluster(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/kubeconfigs?path=%2Ftmp%2Fother.yaml", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if len(cluster.removeCalls()) != 1 || cluster.removeCalls()[0] != "/tmp/other.yaml" {
		t.Fatalf("removed = %v", cluster.removeCalls())
	}
}

func TestRemovingAKubeconfigNeedsAPath(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, _ := doRequest(t, http.MethodDelete, ts.URL+"/api/kubeconfigs", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(cluster.removeCalls()) != 0 {
		t.Fatalf("removed = %v on a bad request", cluster.removeCalls())
	}
}

func TestARemovalTheClusterRefusesIsReported(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	cluster.changeErr = errors.New("spinoza is connected through that kubeconfig")
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodDelete, ts.URL+"/api/kubeconfigs?path=%2Ftmp%2Fother.yaml", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("the kubeconfig in use was removed under the running cluster")
	}
	if !strings.Contains(string(body), "connected through") {
		t.Fatalf("body = %s, want the reason", body)
	}
}

func TestKubeconfigsRejectAGet(t *testing.T) {
	mgr, _ := testManager(t)
	ts := contextServer(t, fixed(mgr))

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/kubeconfigs", nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestTheBrowserBuildOffersNoFileDialog(t *testing.T) {
	mgr, _ := testManager(t)
	ts := contextServer(t, fixed(mgr))

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/kubeconfigs/picker", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	support := decodePicker(t, body)
	if support.Available {
		t.Fatal("a browser tab was offered a native file dialog it cannot open")
	}
	if support.Reason == "" {
		t.Fatal("the picker was refused without saying why")
	}
}

func TestPickingAFileWithoutADialogIsRefused(t *testing.T) {
	mgr, _ := testManager(t)
	ts := contextServer(t, fixed(mgr))

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/kubeconfigs/picker", nil)

	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}
}

func pickerServer(t *testing.T, picker FilePicker) *httptest.Server {
	t.Helper()
	mgr, _ := testManager(t)
	srv := New(fixed(mgr), testAssets(), testToken)
	srv.UseFilePicker(picker)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)
	return ts
}

func TestTheDesktopWindowOpensAFileDialog(t *testing.T) {
	ts := pickerServer(t, func(context.Context) (string, error) {
		return "/home/arch/.kube/other.yaml", nil
	})

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/kubeconfigs/picker", nil)
	if !decodePicker(t, body).Available {
		t.Fatalf("status = %d, support = %s, want the dialog offered", resp.StatusCode, body)
	}

	chosen, chosenBody := doRequest(t, http.MethodPost, ts.URL+"/api/kubeconfigs/picker", nil)

	if chosen.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", chosen.StatusCode, chosenBody)
	}
	var picked api.PickedFile
	if err := json.Unmarshal(chosenBody, &picked); err != nil {
		t.Fatalf("decode %s: %v", chosenBody, err)
	}
	if picked.Path != "/home/arch/.kube/other.yaml" {
		t.Fatalf("path = %q", picked.Path)
	}
}

func TestAFileDialogThatFailsIsReported(t *testing.T) {
	ts := pickerServer(t, func(context.Context) (string, error) {
		return "", errors.New("the spinoza window is not ready yet")
	})

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/kubeconfigs/picker", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a dialog that never opened reported a path")
	}
	if !strings.Contains(string(body), "not ready") {
		t.Fatalf("body = %s, want the reason", body)
	}
}

func decodeContexts(t *testing.T, body []byte) api.ContextList {
	t.Helper()
	var list api.ContextList
	err := json.Unmarshal(body, &list)
	if err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return list
}

func decodePicker(t *testing.T, body []byte) api.FilePicker {
	t.Helper()
	var support api.FilePicker
	err := json.Unmarshal(body, &support)
	if err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return support
}

func TestSwitchingClosesOpenSessions(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "web"))
	cluster := fixed(mgr)
	srv := New(cluster, testAssets(), testToken)
	ts := httptest.NewServer(authed(srv.Handler()))
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	writeErr := wsjson.Write(ctx, conn, api.ClientMsg{
		Type: "subscribe", SubID: "main", Group: "apps", Version: "v1", Resource: "deployments", Namespace: "default",
	})
	if writeErr != nil {
		t.Fatalf("subscribe: %v", writeErr)
	}
	if readMsg(ctx, t, conn).Type != "snapshot" {
		t.Fatal("expected a snapshot")
	}

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/contexts?name=p-mk1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("switch: %d %s", resp.StatusCode, body)
	}

	// The switch announces the new cluster and then drops the socket, so the
	// frames that follow are the announcement and the close.
	for {
		_, payload, readErr := conn.Read(ctx)
		if readErr != nil {
			return
		}
		if !bytes.Contains(payload, []byte(`"type":"context"`)) {
			t.Fatal("the session survived a context switch; it would stream the old cluster's objects")
		}
	}
}

func TestProtectingTheClusterInUse(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/protection?protected=true", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if decodeContexts(t, body).Protection != api.ProtectionProtected {
		t.Fatalf("protection = %q, want it echoed back", decodeContexts(t, body).Protection)
	}
	if !cluster.Protected() {
		t.Fatal("the cluster did not come back protected")
	}
}

func TestOpeningTheClusterUpAgain(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	ts := contextServer(t, cluster)
	doRequest(t, http.MethodPost, ts.URL+"/api/protection?protected=true", nil)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/protection?protected=false", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if cluster.Protected() {
		t.Fatal("the cluster stayed protected")
	}
}

func TestProtectionNeedsAVerdict(t *testing.T) {
	mgr, _ := testManager(t)
	ts := contextServer(t, fixed(mgr))

	for _, query := range []string{"", "?protected=", "?protected=maybe"} {
		resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/protection"+query, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %q = %d, want 400", query, resp.StatusCode)
		}
	}
}

func TestProtectionThatCannotBeSavedIsReported(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	cluster.protectErr = errors.New("read-only file system")
	ts := contextServer(t, cluster)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/protection?protected=true", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatal("a protection that was never written reported success")
	}
	if !strings.Contains(string(body), "read-only") {
		t.Fatalf("body = %s, want the reason", body)
	}
}
