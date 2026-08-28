package flux

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/unstr"
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
	object := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}
	maps.Copy(object, extra)
	return &unstructured.Unstructured{Object: object}
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
	dash := Build(context.Background(), listerFor(dyn), fluxDescs(), nil)

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
	for i, expected := range wantGroups {
		group := dash.Groups[i]
		if group.Name != expected.name {
			t.Fatalf("group %d name = %q, want %q", i, group.Name, expected.name)
		}
		if group.Ready != expected.ready {
			t.Fatalf("group %q ready = %d, want %d", group.Name, group.Ready, expected.ready)
		}
		if group.Total != expected.total {
			t.Fatalf("group %q total = %d, want %d", group.Name, group.Total, expected.total)
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
	dash := Build(context.Background(), listerFor(dyn), descs, nil)
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
	object := obj("x/v1", "X", "n", "ns", map[string]any{
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Stalled", "status": "True"},
			},
		},
	})
	status, message := unstr.Ready(object)
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

func TestBuildReportsAListThatFailed(t *testing.T) {
	dash := Build(context.Background(), listerFor(newClient(t)), fluxDescs(), nil)

	if dash.Error == "" {
		t.Fatal("a failed list was reported as an empty dashboard")
	}
	if !strings.Contains(dash.Error, "buckets") {
		t.Fatalf("error = %q, want it to name the resource", dash.Error)
	}
	if !strings.Contains(dash.Error, "(list buckets failed)") {
		t.Fatalf("error = %q, want the reason", dash.Error)
	}
}

func TestBuildStillReturnsWhatItCouldList(t *testing.T) {
	dash := Build(context.Background(), listerFor(newClient(t)), fluxDescs(), nil)

	if len(dash.Groups) == 0 {
		t.Fatal("one failing list threw away every group")
	}
}

func TestBuildIsSilentWhenEveryListWorks(t *testing.T) {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), fluxObjects()...)

	dash := Build(context.Background(), listerFor(dyn), fluxDescs(), nil)

	if dash.Error != "" {
		t.Fatalf("error = %q, want none", dash.Error)
	}
}

func helmReleaseDesc() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
		Kind:     "HelmRelease",
	}
}

func kustomizationDesc() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
		Kind:     "Kustomization",
	}
}

func withStatus(apiVersion, kind string, status map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": "app", "namespace": "flux-system"},
		"status":     status,
	}}
}

func TestAHelmReleaseReportsTheChartInHelmStorage(t *testing.T) {
	release := withStatus("helm.toolkit.fluxcd.io/v2", "HelmRelease", map[string]any{
		"lastAttemptedRevision": "6.9.0",
		"history": []any{
			map[string]any{"chartVersion": "6.5.0"},
			map[string]any{"chartVersion": "6.4.0"},
		},
	})

	got := revisionOf(release, helmReleaseDesc())

	if got != "6.5.0" {
		t.Fatalf("revision = %q, want the version actually released rather than the one attempted", got)
	}
}

func TestAHelmReleaseWithNoHistoryFallsBackToTheAttempt(t *testing.T) {
	release := withStatus("helm.toolkit.fluxcd.io/v2", "HelmRelease", map[string]any{
		"lastAttemptedRevision": "6.9.0",
	})

	got := revisionOf(release, helmReleaseDesc())

	if got != "6.9.0" {
		t.Fatalf("revision = %q, want the attempt while no release has landed", got)
	}
}

func TestAHelmReleaseIgnoresAMalformedHistoryEntry(t *testing.T) {
	release := withStatus("helm.toolkit.fluxcd.io/v2", "HelmRelease", map[string]any{
		"lastAttemptedRevision": "6.9.0",
		"history":               []any{"not-a-map"},
	})

	got := revisionOf(release, helmReleaseDesc())

	if got != "6.9.0" {
		t.Fatalf("revision = %q", got)
	}
}

func TestAKustomizationStillPrefersTheAppliedRevision(t *testing.T) {
	kustomization := withStatus("kustomize.toolkit.fluxcd.io/v1", "Kustomization", map[string]any{
		"lastAppliedRevision":   "main@sha1:aaa",
		"lastAttemptedRevision": "main@sha1:bbb",
	})

	got := revisionOf(kustomization, kustomizationDesc())

	if got != "main@sha1:aaa" {
		t.Fatalf("revision = %q, want the applied revision for a Kustomization", got)
	}
}

func TestAHelmReleaseDoesNotReadTheKustomizationField(t *testing.T) {
	release := withStatus("helm.toolkit.fluxcd.io/v2", "HelmRelease", map[string]any{
		"lastAppliedRevision": "6.0.0",
		"history": []any{
			map[string]any{"chartVersion": "6.5.0"},
		},
	})

	got := revisionOf(release, helmReleaseDesc())

	if got != "6.5.0" {
		t.Fatalf("revision = %q, want history to win; lastAppliedRevision does not exist on v2", got)
	}
}

func TestWithoutAChartIndexTheRepositoriesAreNotListed(t *testing.T) {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), fluxObjects()...)
	listed := 0
	dyn.PrependReactor("list", "helmrepositories", func(k8stesting.Action) (bool, runtime.Object, error) {
		listed++
		return false, nil, nil
	})

	Build(context.Background(), listerFor(dyn), fluxDescs(), nil)

	if listed != 1 {
		t.Fatalf("listed helmrepositories %d times, want only the dashboard's own list", listed)
	}
}

func TestWithAChartIndexTheRepositoriesAreStillRead(t *testing.T) {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), fluxObjects()...)
	listed := 0
	dyn.PrependReactor("list", "helmrepositories", func(k8stesting.Action) (bool, runtime.Object, error) {
		listed++
		return false, nil, nil
	})

	Build(context.Background(), listerFor(dyn), fluxDescs(), &stubCharts{})

	if listed < 2 {
		t.Fatalf("listed helmrepositories %d times; the chart index needs the repositories", listed)
	}
}

func TestSourceOfNamesAChartRef(t *testing.T) {
	release := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata":   map[string]any{"name": "podinfo", "namespace": "apps"},
		"spec": map[string]any{
			"chartRef": map[string]any{"kind": "OCIRepository", "name": "podinfo-oci"},
		},
	}}

	if got := sourceOf(release); got != "OCIRepository/podinfo-oci" {
		t.Fatalf("source = %q, want the chartRef the graph already follows", got)
	}
}

func TestSourceOfPrefersChartRefOverTheChartBlock(t *testing.T) {
	release := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata":   map[string]any{"name": "podinfo", "namespace": "apps"},
		"spec": map[string]any{
			"chartRef": map[string]any{"kind": "OCIRepository", "name": "podinfo-oci"},
			"chart": map[string]any{
				"spec": map[string]any{
					"sourceRef": map[string]any{"kind": "HelmRepository", "name": "podinfo-charts"},
				},
			},
		},
	}}

	if got := sourceOf(release); got != "OCIRepository/podinfo-oci" {
		t.Fatalf("source = %q", got)
	}
}

func TestSourceOfStillNamesAKustomizationSource(t *testing.T) {
	kustomization := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": "apps", "namespace": "flux-system"},
		"spec": map[string]any{
			"sourceRef": map[string]any{"kind": "GitRepository", "name": "app-repo"},
		},
	}}

	if got := sourceOf(kustomization); got != "GitRepository/app-repo" {
		t.Fatalf("source = %q", got)
	}
}

func helmReleaseNaming(repoNamespace, repoName string) *unstructured.Unstructured {
	sourceRef := map[string]any{"kind": "HelmRepository", "name": repoName}
	if repoNamespace != "" {
		sourceRef["namespace"] = repoNamespace
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata":   map[string]any{"name": "podinfo", "namespace": "apps"},
		"spec": map[string]any{
			"chart": map[string]any{
				"spec": map[string]any{"chart": "podinfo", "sourceRef": sourceRef},
			},
		},
	}}
}

func TestAChartWhoseRepositoryIsNotThereHasNoSource(t *testing.T) {
	_, _, ok := chartSource(helmReleaseNaming("", "missing"), map[string]charts.Repo{})

	if ok {
		t.Fatal("a chart pointing at a repository nobody has was given a source")
	}
}

func TestAChartWhoseRepositoryHasNoUrlHasNoSource(t *testing.T) {
	repos := map[string]charts.Repo{"apps/podinfo": {}}

	_, _, ok := chartSource(helmReleaseNaming("", "podinfo"), repos)

	if ok {
		t.Fatal("a repository with no url was offered as a source")
	}
}

func TestAChartTakesTheRepositoryFromItsOwnNamespaceByDefault(t *testing.T) {
	repos := map[string]charts.Repo{"apps/podinfo": {URL: "https://example.test"}}

	repo, chart, ok := chartSource(helmReleaseNaming("", "podinfo"), repos)

	if !ok {
		t.Fatal("a repository in the release's own namespace was not found")
	}
	if chart != "podinfo" || repo.URL != "https://example.test" {
		t.Fatalf("chart = %q repo = %+v", chart, repo)
	}
}

func TestAChartCanNameARepositoryInAnotherNamespace(t *testing.T) {
	repos := map[string]charts.Repo{"flux-system/podinfo": {URL: "https://example.test"}}

	repo, _, ok := chartSource(helmReleaseNaming("flux-system", "podinfo"), repos)

	if !ok {
		t.Fatal("a repository named with its namespace was not found")
	}
	if repo.URL != "https://example.test" {
		t.Fatalf("repo = %+v", repo)
	}
}
