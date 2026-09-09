package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const rolledQuery = "?group=apps&version=v1&resource=deployments&namespace=prod&name=web"

func rolledTemplate(image, hash string) map[string]any {
	labels := map[string]any{"app": "web"}
	if hash != "" {
		labels["pod-template-hash"] = hash
	}
	return map[string]any{
		"metadata": map[string]any{"labels": labels},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{"name": "app", "image": image},
			},
		},
	}
}

func rolledDeployment() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":            "web",
			"namespace":       "prod",
			"uid":             "web-uid",
			"resourceVersion": "1",
		},
		"spec": map[string]any{
			"replicas": int64(2),
			"template": rolledTemplate("nginx:2.0", ""),
		},
	}}
}

func rolledReplicaSet(name, revision, image string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata": map[string]any{
			"name":            name,
			"namespace":       "prod",
			"resourceVersion": "1",
			"annotations":     map[string]any{"deployment.kubernetes.io/revision": revision},
			"ownerReferences": []any{
				map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "web", "uid": "web-uid"},
			},
		},
		"spec": map[string]any{
			"replicas": int64(2),
			"template": rolledTemplate(image, name),
		},
	}}
}

func rolloutServer(t *testing.T, objs ...runtime.Object) *httptest.Server {
	t.Helper()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "replicasets"}: "ReplicaSetList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objs...)
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("apps", "v1", "deployments"): {
			Group:      "apps",
			Version:    "v1",
			Resource:   "deployments",
			Kind:       "Deployment",
			Namespaced: true,
		},
	}
	mgr := resources.NewManager(t.Context(), resources.Deps{Dynamic: dyn, Clientset: k8sfake.NewClientset(), Descriptors: descs})
	return clusterServer(t, fixed(mgr))
}

func rolledWorld() []runtime.Object {
	return []runtime.Object{
		rolledDeployment(),
		rolledReplicaSet("web-1", "1", "nginx:1.0"),
		rolledReplicaSet("web-2", "2", "nginx:2.0"),
	}
}

func revisionsFrom(t *testing.T, body []byte) api.Revisions {
	t.Helper()
	var found api.Revisions
	err := json.Unmarshal(body, &found)
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	return found
}

func revisionDiffFrom(t *testing.T, body []byte) api.RevisionDiff {
	t.Helper()
	var diff api.RevisionDiff
	err := json.Unmarshal(body, &diff)
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	return diff
}

func refusalFrom(t *testing.T, body []byte) string {
	t.Helper()
	var failure api.Failure
	err := json.Unmarshal(body, &failure)
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	return failure.Message
}

func TestTheRolloutEndpointAnswersWithTheRevisionsNewestFirst(t *testing.T) {
	ts := rolloutServer(t, rolledWorld()...)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/rollout"+rolledQuery, nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	found := revisionsFrom(t, body)
	if !found.Supported {
		t.Fatalf("supported = false, want a deployment to keep revisions: %s", body)
	}
	if len(found.Revisions) != 2 || found.Revisions[0].Number != 2 || found.Revisions[1].Number != 1 {
		t.Fatalf("revisions = %+v, want 2 then 1", found.Revisions)
	}
	if !found.Revisions[0].Current {
		t.Fatal("the revision matching the live pod template is not marked current")
	}
}

func TestTheRolloutEndpointSaysWhenAKindKeepsNoRevisions(t *testing.T) {
	ts := rolloutServer(t, rolledWorld()...)

	resp, body := doRequest(t, http.MethodGet,
		ts.URL+"/api/rollout?version=v1&resource=configmaps&namespace=prod&name=settings", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	found := revisionsFrom(t, body)
	if found.Supported {
		t.Fatal("supported = true, want a config map to say it keeps no revisions")
	}
	if found.Reason != "configmaps keep no rollout revisions" {
		t.Fatalf("reason = %q", found.Reason)
	}
}

func TestTheRolloutEndpointReportsAWorkloadThatIsNotThere(t *testing.T) {
	ts := rolloutServer(t)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/rollout"+rolledQuery, nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, body)
	}
}

func TestTheDiffEndpointRendersBothRevisions(t *testing.T) {
	ts := rolloutServer(t, rolledWorld()...)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/rollout/diff"+rolledQuery+"&from=1&to=2", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	diff := revisionDiffFrom(t, body)
	if diff.From != 1 || diff.To != 2 {
		t.Fatalf("from = %d to = %d, want the revisions that were asked for", diff.From, diff.To)
	}
	if diff.Same || diff.Lines == 0 {
		t.Fatalf("same = %v lines = %d, want a changed image to show", diff.Same, diff.Lines)
	}
	if !strings.Contains(diff.Left, "nginx:1.0") || !strings.Contains(diff.Right, "nginx:2.0") {
		t.Fatalf("left = %q right = %q", diff.Left, diff.Right)
	}
}

func TestTheDiffEndpointNeedsTwoRevisionNumbers(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{name: "neither side is named", query: "", want: "from must be a revision number"},
		{name: "the right side is missing", query: "&from=1", want: "to must be a revision number"},
		{name: "the left side is not a number", query: "&from=latest&to=2", want: "from must be a revision number"},
		{name: "the right side is not a number", query: "&from=1&to=latest", want: "to must be a revision number"},
	}
	ts := rolloutServer(t, rolledWorld()...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/rollout/diff"+rolledQuery+tc.query, nil)

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode, body)
			}
			if refusalFrom(t, body) != tc.want {
				t.Fatalf("message = %q, want %q", refusalFrom(t, body), tc.want)
			}
		})
	}
}

func TestTheDiffEndpointReportsARevisionThatIsNotThere(t *testing.T) {
	ts := rolloutServer(t, rolledWorld()...)

	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/rollout/diff"+rolledQuery+"&from=1&to=9", nil)

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("status = %d, want a refusal: %s", resp.StatusCode, body)
	}
	if refusalFrom(t, body) != "web has no revision 9" {
		t.Fatalf("message = %q, want it to name the revision", refusalFrom(t, body))
	}
}
