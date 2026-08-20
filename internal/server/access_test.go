package server

import (
	"context"
	"encoding/json"
	"net/http"
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

func TestAccessIsReadOnly(t *testing.T) {
	ts := inspectServerWith(t, decidingClient(t, true, ""), newPod())

	resp, _ := doRequest(t, http.MethodPost, ts.URL+"/api/access"+objectQuery, nil)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
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

func TestSwitchingContextTellsEveryOpenFeed(t *testing.T) {
	ts := inspectServer(t, newPod())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	readAnyMsg(ctx, t, conn)

	resp, body := doRequest(t, http.MethodPost, ts.URL+"/api/contexts?name=elsewhere", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}

	told := nextContextFrame(ctx, t, conn)
	if told.Type != "context" {
		t.Fatalf("type = %q, want a context frame on the switch", told.Type)
	}
	if told.Context != "elsewhere" {
		t.Fatalf("context = %q, want the cluster that was switched to", told.Context)
	}
}

// nextContextFrame skips past whatever else the server is saying to find the
// next word about which cluster it is on.
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
