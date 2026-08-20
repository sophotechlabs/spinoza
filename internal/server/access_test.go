package server

import (
	"encoding/json"
	"net/http"
	"testing"

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
