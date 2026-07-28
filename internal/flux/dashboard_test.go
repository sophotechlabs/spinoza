package flux

import (
	"context"
	"errors"
	"maps"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

var (
	kustGVR      = schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	helmGVR      = schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
	gitRepoGVR   = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	helmRepoGVR  = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories"}
	bucketGVR    = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "buckets"}
	imgPolicyGVR = schema.GroupVersionResource{Group: "image.toolkit.fluxcd.io", Version: "v1beta2", Resource: "imagepolicies"}
	imgRepoGVR   = schema.GroupVersionResource{Group: "image.toolkit.fluxcd.io", Version: "v1beta2", Resource: "imagerepositories"}
)

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		kustGVR:      "KustomizationList",
		helmGVR:      "HelmReleaseList",
		gitRepoGVR:   "GitRepositoryList",
		helmRepoGVR:  "HelmRepositoryList",
		bucketGVR:    "BucketList",
		imgPolicyGVR: "ImagePolicyList",
		imgRepoGVR:   "ImageRepositoryList",
	}
}

func desc(group, version, resource, kind string) api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      group,
		Version:    version,
		Resource:   resource,
		Kind:       kind,
		Namespaced: true,
		Category:   "Custom Resources",
	}
}

func fluxDescs() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		discovery.Key("kustomize.toolkit.fluxcd.io", "v1", "kustomizations"):     desc("kustomize.toolkit.fluxcd.io", "v1", "kustomizations", "Kustomization"),
		discovery.Key("helm.toolkit.fluxcd.io", "v2", "helmreleases"):            desc("helm.toolkit.fluxcd.io", "v2", "helmreleases", "HelmRelease"),
		discovery.Key("source.toolkit.fluxcd.io", "v1", "gitrepositories"):       desc("source.toolkit.fluxcd.io", "v1", "gitrepositories", "GitRepository"),
		discovery.Key("source.toolkit.fluxcd.io", "v1", "helmrepositories"):      desc("source.toolkit.fluxcd.io", "v1", "helmrepositories", "HelmRepository"),
		discovery.Key("source.toolkit.fluxcd.io", "v1", "buckets"):               desc("source.toolkit.fluxcd.io", "v1", "buckets", "Bucket"),
		discovery.Key("image.toolkit.fluxcd.io", "v1beta2", "imagepolicies"):     desc("image.toolkit.fluxcd.io", "v1beta2", "imagepolicies", "ImagePolicy"),
		discovery.Key("image.toolkit.fluxcd.io", "v1beta2", "imagerepositories"): desc("image.toolkit.fluxcd.io", "v1beta2", "imagerepositories", "ImageRepository"),
		discovery.Key("apps", "v1", "deployments"):                               desc("apps", "v1", "deployments", "Deployment"),
	}
}

func obj(apiVersion, kind, name, namespace string, extra map[string]any) *unstructured.Unstructured {
	o := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}
	maps.Copy(o, extra)
	return &unstructured.Unstructured{Object: o}
}

func fluxObjects() []runtime.Object {
	return []runtime.Object{
		obj("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "apps", "flux-system", map[string]any{
			"spec": map[string]any{
				"sourceRef": map[string]any{"kind": "GitRepository", "name": "app-repo"},
			},
			"status": map[string]any{
				"lastAppliedRevision": "main@sha1:abc123",
				"conditions": []any{
					map[string]any{"type": "Ready", "status": "True", "message": "Applied revision: main@sha1:abc123"},
				},
			},
		}),
		obj("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "infra", "flux-system", map[string]any{
			"spec": map[string]any{"suspend": true},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{"type": "Ready", "status": "False", "message": "build failed"},
				},
			},
		}),
		obj("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "tenant", "team-a", map[string]any{}),
		obj("helm.toolkit.fluxcd.io/v2", "HelmRelease", "podinfo", "flux-system", map[string]any{
			"spec": map[string]any{
				"chart": map[string]any{
					"spec": map[string]any{
						"sourceRef": map[string]any{"kind": "HelmRepository", "name": "podinfo"},
					},
				},
			},
			"status": map[string]any{
				"lastAttemptedRevision": "6.0.0",
				"conditions": []any{
					"not-a-map",
					map[string]any{"type": "Stalled", "status": "False"},
					map[string]any{"type": "Ready", "status": "False", "message": "install retries exhausted"},
				},
			},
		}),
		obj("source.toolkit.fluxcd.io/v1", "GitRepository", "app-repo", "flux-system", map[string]any{
			"status": map[string]any{
				"artifact": map[string]any{"revision": "main@sha1:def456"},
				"conditions": []any{
					map[string]any{"type": "Ready", "status": "True", "message": "stored artifact"},
				},
			},
		}),
		obj("source.toolkit.fluxcd.io/v1", "HelmRepository", "podinfo", "flux-system", map[string]any{
			"status": map[string]any{
				"artifact": map[string]any{"revision": "sha256:aaa"},
				"conditions": []any{
					map[string]any{"type": "Ready", "status": "True"},
				},
			},
		}),
		obj("image.toolkit.fluxcd.io/v1beta2", "ImagePolicy", "app", "flux-system", map[string]any{
			"status": map[string]any{
				"latestImage": "ghcr.io/org/app:1.2.3",
				"conditions": []any{
					map[string]any{"type": "Ready", "status": "True"},
				},
			},
		}),
		obj("image.toolkit.fluxcd.io/v1beta2", "ImageRepository", "app", "flux-system", map[string]any{
			"status": map[string]any{},
		}),
	}
}

func newClient(t *testing.T) *fake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds(), fluxObjects()...)
	dyn.PrependReactor("list", "buckets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("list buckets failed")
	})
	return dyn
}

func TestBuild(t *testing.T) {
	dyn := newClient(t)
	dash := Build(context.Background(), dyn, fluxDescs(), nil)

	wantGroups := []struct {
		name  string
		ready int
		total int
	}{
		{"Kustomizations", 1, 3},
		{"Helm Releases", 0, 1},
		{"Sources", 2, 2},
		{"Image Automation", 1, 2},
	}
	if len(dash.Groups) != len(wantGroups) {
		t.Fatalf("groups = %d, want %d", len(dash.Groups), len(wantGroups))
	}
	for i, w := range wantGroups {
		g := dash.Groups[i]
		if g.Name != w.name {
			t.Fatalf("group %d name = %q, want %q", i, g.Name, w.name)
		}
		if g.Ready != w.ready {
			t.Fatalf("group %q ready = %d, want %d", g.Name, g.Ready, w.ready)
		}
		if g.Total != w.total {
			t.Fatalf("group %q total = %d, want %d", g.Name, g.Total, w.total)
		}
	}

	byKey := map[string]api.FluxResource{}
	for _, g := range dash.Groups {
		for _, r := range g.Resources {
			byKey[r.Kind+"/"+r.Namespace+"/"+r.Name] = r
		}
	}

	apps := byKey["Kustomization/flux-system/apps"]
	if apps.Ready != "True" || apps.Suspended || apps.Revision != "main@sha1:abc123" || apps.Source != "GitRepository/app-repo" || apps.Message != "Applied revision: main@sha1:abc123" {
		t.Fatalf("apps kustomization = %+v", apps)
	}
	infra := byKey["Kustomization/flux-system/infra"]
	if infra.Ready != "False" || !infra.Suspended || infra.Message != "build failed" {
		t.Fatalf("infra kustomization = %+v", infra)
	}
	podinfo := byKey["HelmRelease/flux-system/podinfo"]
	if podinfo.Ready != "False" || podinfo.Revision != "6.0.0" || podinfo.Source != "HelmRepository/podinfo" || podinfo.Message != "install retries exhausted" {
		t.Fatalf("podinfo helmrelease = %+v", podinfo)
	}
	gitRepo := byKey["GitRepository/flux-system/app-repo"]
	if gitRepo.Ready != "True" || gitRepo.Revision != "main@sha1:def456" || gitRepo.Source != "" {
		t.Fatalf("gitrepository = %+v", gitRepo)
	}
	helmRepo := byKey["HelmRepository/flux-system/podinfo"]
	if helmRepo.Revision != "sha256:aaa" || helmRepo.Message != "" {
		t.Fatalf("helmrepository = %+v", helmRepo)
	}
	policy := byKey["ImagePolicy/flux-system/app"]
	if policy.Revision != "ghcr.io/org/app:1.2.3" {
		t.Fatalf("imagepolicy = %+v", policy)
	}
	imgRepo := byKey["ImageRepository/flux-system/app"]
	if imgRepo.Ready != "" || imgRepo.Revision != "" {
		t.Fatalf("imagerepository = %+v", imgRepo)
	}

	kustGroup := dash.Groups[0]
	order := []string{
		kustGroup.Resources[0].Namespace + "/" + kustGroup.Resources[0].Name,
		kustGroup.Resources[1].Namespace + "/" + kustGroup.Resources[1].Name,
		kustGroup.Resources[2].Namespace + "/" + kustGroup.Resources[2].Name,
	}
	want := []string{"flux-system/apps", "flux-system/infra", "team-a/tenant"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("kustomization order = %v, want %v", order, want)
		}
	}
}

func TestBuildEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds())
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("apps", "v1", "deployments"): desc("apps", "v1", "deployments", "Deployment"),
	}
	dash := Build(context.Background(), dyn, descs, nil)
	if len(dash.Groups) != 0 {
		t.Fatalf("groups = %d, want 0", len(dash.Groups))
	}
}

func TestCategoryOf(t *testing.T) {
	cases := []struct {
		group    string
		resource string
		want     string
	}{
		{"kustomize.toolkit.fluxcd.io", "kustomizations", "Kustomizations"},
		{"kustomize.toolkit.fluxcd.io", "other", ""},
		{"helm.toolkit.fluxcd.io", "helmreleases", "Helm Releases"},
		{"helm.toolkit.fluxcd.io", "other", ""},
		{"source.toolkit.fluxcd.io", "gitrepositories", "Sources"},
		{"source.toolkit.fluxcd.io", "helmrepositories", "Sources"},
		{"source.toolkit.fluxcd.io", "ocirepositories", "Sources"},
		{"source.toolkit.fluxcd.io", "buckets", "Sources"},
		{"source.toolkit.fluxcd.io", "other", ""},
		{"image.toolkit.fluxcd.io", "imagerepositories", "Image Automation"},
		{"image.toolkit.fluxcd.io", "imagepolicies", "Image Automation"},
		{"image.toolkit.fluxcd.io", "imageupdateautomations", "Image Automation"},
		{"image.toolkit.fluxcd.io", "other", ""},
		{"notification.toolkit.fluxcd.io", "alerts", "Notifications"},
		{"notification.toolkit.fluxcd.io", "providers", "Notifications"},
		{"notification.toolkit.fluxcd.io", "receivers", "Notifications"},
		{"notification.toolkit.fluxcd.io", "other", ""},
		{"apps", "deployments", ""},
	}
	for _, c := range cases {
		got := categoryOf(desc(c.group, "v1", c.resource, "X"))
		if got != c.want {
			t.Fatalf("categoryOf(%s/%s) = %q, want %q", c.group, c.resource, got, c.want)
		}
	}
}

func TestReadyConditionNoReady(t *testing.T) {
	u := obj("x/v1", "X", "n", "ns", map[string]any{
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Stalled", "status": "True"},
			},
		},
	})
	status, message := readyCondition(u)
	if status != "" || message != "" {
		t.Fatalf("readyCondition = %q,%q, want empty", status, message)
	}
}

func TestReportingCountExcludesResourcesWithoutAReadyCondition(t *testing.T) {
	items := []api.FluxResource{
		{Ready: "True"},
		{Ready: "False"},
		{Ready: ""},
		{Ready: "Unknown"},
	}
	if got := readyCount(items); got != 1 {
		t.Fatalf("readyCount = %d, want 1", got)
	}
	if got := reportingCount(items); got != 3 {
		t.Fatalf("reportingCount = %d, want 3", got)
	}
}
