package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func decidingClient(t *testing.T, allowed bool, reason string) *k8sfake.Clientset {
	t.Helper()
	cs := k8sfake.NewClientset()
	cs.PrependReactor(
		"create",
		"selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			create, ok := action.(k8stesting.CreateAction)
			if !ok {
				return false, nil, nil
			}
			review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
			if !ok {
				return false, nil, nil
			}
			review.Status = authv1.SubjectAccessReviewStatus{Allowed: allowed, Reason: reason}
			return true, review, nil
		},
	)
	return cs
}

func accessOf(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var result api.Access
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	out := map[string]string{}
	for _, refusal := range result.Refused {
		out[refusal.Capability] = refusal.Reason
	}
	return out
}

func TestAccessReportsWhatTheClusterRefuses(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, false, "no such luck"), newPod())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/access"+objectQuery, nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	refused := accessOf(t, body)
	for _, capability := range []string{"logs", "exec", "portForward", "delete", "edit"} {
		if refused[capability] != "no such luck" {
			t.Fatalf("%s = %q, want the cluster's reason", capability, refused[capability])
		}
	}
}

func TestAccessHoldsNothingBackWhenEverythingIsAllowed(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/access"+objectQuery, nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if len(accessOf(t, body)) != 0 {
		t.Fatalf("refused = %v, want nothing", accessOf(t, body))
	}
}

func TestAccessNeedsAnObject(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, _ := doRequest(t, http.MethodGet, ts.URL+"/api/access?version=v1&resource=pods", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAccessCannotBeChanged(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, _ := doRequest(t, http.MethodPut, ts.URL+"/api/access"+objectQuery, nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func bulkQuery(capability string, names ...string) string {
	refs := make([]api.ObjectRef, 0, len(names))
	for _, name := range names {
		refs = append(refs, api.ObjectRef{
			Version:   "v1",
			Resource:  "pods",
			Namespace: "flux-system",
			Name:      name,
		})
	}
	body, err := json.Marshal(api.AccessQuery{Capability: capability, Refs: refs})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func bulkAccessOf(t *testing.T, body []byte) api.BulkAccess {
	t.Helper()
	var result api.BulkAccess
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	return result
}

func askAccess(t *testing.T, ts *httptest.Server, body string) (*http.Response, []byte) {
	t.Helper()
	return doRequest(t, http.MethodPost, ts.URL+"/api/access", strings.NewReader(body))
}

func TestASelectionIsAnsweredRowByRow(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, false, "no deleting here"), newPod())

	resp, body := askAccess(t, ts, bulkQuery("delete", "web-0", "web-1"))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	result := bulkAccessOf(t, body)
	if len(result.Refused) != 2 {
		t.Fatalf("refused = %v, want both rows", result.Refused)
	}
	if result.Refused[0].At != 0 || result.Refused[1].At != 1 {
		t.Fatalf("refused = %v, want the rows named by their place", result.Refused)
	}
	if result.Refused[0].Reason != "no deleting here" {
		t.Fatalf("reason = %q, want the cluster's own words", result.Refused[0].Reason)
	}
}

func TestASelectionThePermittedIsAnsweredWithNothing(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, body := askAccess(t, ts, bulkQuery("delete", "web-0", "web-1"))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if len(bulkAccessOf(t, body).Refused) != 0 {
		t.Fatalf("refused = %s, want nothing", body)
	}
}

func TestASelectionQueryThatIsNotJsonIsRefused(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, body := askAccess(t, ts, "{")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestASelectionQueryNeedsACapability(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, body := askAccess(t, ts, bulkQuery("", "web-0"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestASelectionQueryNeedsEveryObjectNamed(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())
	body := `{"capability":"delete","refs":[{"version":"v1","resource":"pods","namespace":"prod"}]}`

	resp, said := askAccess(t, ts, body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, said)
	}
	if !strings.Contains(string(said), "name") {
		t.Fatalf("message = %s, want it to say what is missing", said)
	}
}

func TestASelectionLargerThanTheCapIsRefused(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())
	names := make([]string, 0, maxAccessRefs+1)
	for i := range maxAccessRefs + 1 {
		names = append(names, fmt.Sprintf("web-%d", i))
	}

	resp, body := askAccess(t, ts, bulkQuery("delete", names...))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), strconv.Itoa(maxAccessRefs)) {
		t.Fatalf("message = %s, want it to say how many is too many", body)
	}
}

func TestASelectionOfNothingIsAnsweredWithNothing(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, body := askAccess(t, ts, bulkQuery("delete"))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if len(bulkAccessOf(t, body).Refused) != 0 {
		t.Fatalf("refused = %s", body)
	}
}

func TestAFeedIsToldWhichClusterItIsOn(t *testing.T) {
	ts := inspectServer(t, newPod())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	hello := readAnyMsg(ctx, t, conn)

	if hello.Type != "context" {
		t.Fatalf("type = %q, want the context frame first", hello.Type)
	}
	if hello.Context == "" {
		t.Fatal("the context frame named no cluster")
	}
}

func TestBringingATabForwardTellsEveryOpenFeed(t *testing.T) {
	ts := inspectServer(t, newPod())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	readAnyMsg(ctx, t, conn)

	resp, body := doRequest(t, http.MethodPost,
		ts.URL+"/api/clusters/active?cluster="+urlValue("https://p-mk2:6443"), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}

	told := nextContextFrame(ctx, t, conn)
	if told.Type != "context" {
		t.Fatalf("type = %q, want a context frame when the tab in front changes", told.Type)
	}
	if told.Context == "" {
		t.Fatalf("frame = %+v, want it to name the cluster now in front", told)
	}
}

func nextContextFrame(ctx context.Context, t *testing.T, conn *websocket.Conn) api.ServerMsg {
	t.Helper()
	for range 10 {
		msg := readAnyMsg(ctx, t, conn)
		if msg.Type == "context" {
			return msg
		}
	}
	t.Fatal("no context frame arrived")
	return api.ServerMsg{}
}

func TestComparingAKindNeedsAKind(t *testing.T) {
	ts := inspectServer(t, newPod())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/compare/kind?version=v1&against=p-mk1", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestComparingAKindNeedsSomewhereToCompareWith(t *testing.T) {
	ts := inspectServer(t, newPod())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/compare/kind?version=v1&resource=pods", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestComparingAKindThatCannotBeListedHere(t *testing.T) {
	ts := inspectServer(t, newPod())

	resp, body := doRequest(
		t,
		http.MethodGet,
		ts.URL+"/api/compare/kind?version=v1&resource=nothings&against=p-mk1",
		nil,
	)

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("a kind that cannot be listed compared fine: %s", body)
	}
}

func TestInstallingAChartNeedsEveryField(t *testing.T) {
	ts := inspectServer(t, newPod())

	resp, body := doRequest(
		t,
		http.MethodPost,
		ts.URL+"/api/helm/install",
		strings.NewReader(`{"namespace":"prod"}`),
	)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestAnInstallRequestThatIsNotJsonIsRefused(t *testing.T) {
	ts := inspectServer(t, newPod())

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/helm/install", strings.NewReader("{"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestInstallingAChartOnAProtectedClusterNeedsTheName(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	cluster.protection = api.ProtectionProtected
	ts := httptest.NewServer(authed(New(cluster, testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	body := `{"namespace":"prod","name":"podinfo","chart":"podinfo","repo":"https://example.test","version":"6.7.1"}`

	resp, said := doRequest(t, http.MethodPost, ts.URL+"/api/helm/install", strings.NewReader(body))

	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want the typed confirmation demanded: %s", resp.StatusCode, said)
	}
	if !strings.Contains(string(said), "podinfo") {
		t.Fatalf("message = %s, want it to name what to type", said)
	}
}

func TestADryRunInstallGoesStraightThrough(t *testing.T) {
	mgr, _ := testManager(t)
	cluster := fixed(mgr)
	cluster.protection = api.ProtectionProtected
	ts := httptest.NewServer(authed(New(cluster, testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)
	body := `{"namespace":"prod","name":"podinfo","chart":"podinfo","repo":"https://example.test","version":"6.7.1"}`

	resp, _ := doRequest(
		t,
		http.MethodPost,
		ts.URL+"/api/helm/install?dryRun=true",
		strings.NewReader(body),
	)

	if resp.StatusCode == http.StatusPreconditionFailed {
		t.Fatal("a dry run was made to type the release name")
	}
}

func TestApplyingADocumentWithNoResourceVersionIsABadRequest(t *testing.T) {
	ts := inspectServer(t, newPod())
	doc := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n  namespace: flux-system\n"

	resp, body := doRequest(t, http.MethodPut, ts.URL+"/api/object"+objectQuery, strings.NewReader(doc))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "resourceVersion") {
		t.Fatalf("message = %s, want it to name what is missing", body)
	}
}

func TestComparingAKindTheOtherClusterCannotList(t *testing.T) {
	mgr, _ := testManager(t, newDeployment("default", "api"))
	cluster := fixed(mgr)
	cluster.listErr = errors.New("that cluster is unreachable")
	ts := httptest.NewServer(authed(New(cluster, testAssets(), testToken).Handler()))
	t.Cleanup(ts.Close)

	resp, body := doRequest(
		t,
		http.MethodGet,
		ts.URL+"/api/compare/kind?group=apps&version=v1&resource=deployments&namespace=default&against=p-mk1",
		nil,
	)

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("a far side that could not be listed compared fine: %s", body)
	}
	if !strings.Contains(string(body), "unreachable") {
		t.Fatalf("message = %s, want what went wrong", body)
	}
}

// A client that hung up mid-response.
type deafWriter struct {
	header http.Header
	status int
	err    error
}

func (d *deafWriter) Header() http.Header {
	if d.header == nil {
		d.header = http.Header{}
	}
	return d.header
}

func (d *deafWriter) Write([]byte) (int, error) {
	return 0, d.err
}

func (d *deafWriter) WriteHeader(status int) {
	d.status = status
}

func TestAResponseNobodyIsListeningForIsNotFatal(t *testing.T) {
	writer := &deafWriter{err: errors.New("client disconnected")}

	writeJSONStatus(writer, http.StatusCreated, api.Build{Version: "v1.11.0"})

	if writer.status != http.StatusCreated {
		t.Fatalf("status = %d, want it written before the body failed", writer.status)
	}
	if writer.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", writer.Header().Get("Content-Type"))
	}
}

func TestAnErrorNobodyIsListeningForIsNotFatal(t *testing.T) {
	writer := &deafWriter{err: errors.New("client disconnected")}

	writeError(writer, http.StatusBadRequest, "version, resource and name are required")

	if writer.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want the code written before the body failed", writer.status)
	}
}

func TestHelmAccessReportsWhatTheClusterRefuses(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, false, "no writing here"), newPod())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/helm/access?namespace=demo&name=podinfo", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	refused := accessOf(t, body)
	for _, capability := range []string{"install", "upgrade", "rollback", "uninstall"} {
		if refused[capability] != "no writing here" {
			t.Fatalf("%s = %q, want the cluster's reason", capability, refused[capability])
		}
	}
}

func TestHelmAccessHoldsNothingBackWhenEverythingIsAllowed(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/helm/access?namespace=demo&name=podinfo", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if len(accessOf(t, body)) != 0 {
		t.Fatalf("refused = %v, want nothing", accessOf(t, body))
	}
}

func TestHelmAccessAnswersWithoutAReleaseName(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, false, "no writing here"), newPod())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/helm/access?namespace=demo", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if accessOf(t, body)["install"] != "no writing here" {
		t.Fatalf("refused = %v, want the install held back", accessOf(t, body))
	}
}

func TestHelmAccessNeedsANamespace(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/helm/access?name=podinfo", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "namespace") {
		t.Fatalf("message = %s, want it to say what is missing", body)
	}
}

func TestHelmAccessCannotBeChanged(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/helm/access?namespace=demo", nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}
