package flux

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

type stubCharts struct {
	versions map[string]string
	warmed   []string
}

func (s *stubCharts) Latest(repo charts.Repo, chart string) string {
	return s.versions[repo.URL+"|"+chart]
}

func (s *stubCharts) Warm(repo charts.Repo, chart string) {
	s.warmed = append(s.warmed, repo.URL+"|"+chart)
}

var (
	releaseGVR = schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}
	repoGVR = schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "helmrepositories",
	}
)

func latestDescs() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		discovery.Key("helm.toolkit.fluxcd.io", "v2", "helmreleases"): {
			Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases",
			Kind: "HelmRelease", Namespaced: true,
		},
		discovery.Key("source.toolkit.fluxcd.io", "v1", "helmrepositories"): {
			Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories",
			Kind: "HelmRepository", Namespaced: true,
		},
	}
}

func newRepo(name, url, repoType string) *unstructured.Unstructured {
	spec := map[string]any{"url": url}
	if repoType != "" {
		spec["type"] = repoType
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "HelmRepository",
		"metadata":   map[string]any{"name": name, "namespace": "flux-system"},
		"spec":       spec,
	}}
}

func newRelease(namespace, name, chart, sourceName, sourceNs, revision string) *unstructured.Unstructured {
	sourceRef := map[string]any{"kind": "HelmRepository", "name": sourceName}
	if sourceNs != "" {
		sourceRef["namespace"] = sourceNs
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec": map[string]any{
			"chart": map[string]any{
				"spec": map[string]any{"chart": chart, "sourceRef": sourceRef},
			},
		},
		"status": map[string]any{"lastAppliedRevision": revision},
	}}
}

func latestClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{
		releaseGVR: "HelmReleaseList",
		repoGVR:    "HelmRepositoryList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func releaseRow(t *testing.T, dash api.FluxDashboard, name string) api.FluxResource {
	t.Helper()
	for _, group := range dash.Groups {
		if group.Name != "Helm Releases" {
			continue
		}
		for _, resource := range group.Resources {
			if resource.Name == name {
				return resource
			}
		}
	}
	t.Fatalf("release %q not found in %+v", name, dash.Groups)
	return api.FluxResource{}
}

func TestBuildMarksAnOutdatedRelease(t *testing.T) {
	client := latestClient(
		newRepo("podinfo", "https://example.test/charts", ""),
		newRelease("apps", "podinfo", "podinfo", "podinfo", "flux-system", "6.14.0"),
	)
	index := &stubCharts{versions: map[string]string{"https://example.test/charts|podinfo": "6.15.1"}}

	dash := Build(context.Background(), client, latestDescs(), index)

	row := releaseRow(t, dash, "podinfo")
	if row.Latest != "6.15.1" {
		t.Fatalf("latest = %q, want 6.15.1", row.Latest)
	}
	if !row.Outdated {
		t.Fatalf("outdated = false, want true")
	}
	if len(index.warmed) != 1 || index.warmed[0] != "https://example.test/charts|podinfo" {
		t.Fatalf("warmed = %v", index.warmed)
	}
}

func TestBuildLeavesACurrentReleaseUnmarked(t *testing.T) {
	client := latestClient(
		newRepo("podinfo", "https://example.test/charts", ""),
		newRelease("apps", "podinfo", "podinfo", "podinfo", "flux-system", "6.15.1"),
	)
	index := &stubCharts{versions: map[string]string{"https://example.test/charts|podinfo": "6.15.1"}}

	row := releaseRow(t, Build(context.Background(), client, latestDescs(), index), "podinfo")

	if row.Latest != "6.15.1" {
		t.Fatalf("latest = %q", row.Latest)
	}
	if row.Outdated {
		t.Fatalf("outdated = true for an up-to-date release")
	}
}

func TestBuildDefaultsTheSourceNamespaceToTheRelease(t *testing.T) {
	client := latestClient(
		newRepo("podinfo", "https://example.test/charts", ""),
		newRelease("flux-system", "podinfo", "podinfo", "podinfo", "", "6.14.0"),
	)
	index := &stubCharts{versions: map[string]string{"https://example.test/charts|podinfo": "6.15.1"}}

	row := releaseRow(t, Build(context.Background(), client, latestDescs(), index), "podinfo")

	if row.Latest != "6.15.1" {
		t.Fatalf("latest = %q, want the same-namespace source to resolve", row.Latest)
	}
}

func TestBuildFlagsOCIRepositories(t *testing.T) {
	client := latestClient(
		newRepo("keycloak", "oci://registry.test/team", "oci"),
		newRelease("keycloak", "keycloak", "keycloak", "keycloak", "flux-system", "0.21.14"),
	)
	index := &stubCharts{versions: map[string]string{"oci://registry.test/team|keycloak": "0.22.0"}}

	dash := Build(context.Background(), client, latestDescs(), index)

	if !releaseRow(t, dash, "keycloak").Outdated {
		t.Fatalf("expected the oci-sourced release to be marked outdated")
	}
}

func TestBuildSkipsReleasesWithoutAResolvableSource(t *testing.T) {
	cases := map[string]*unstructured.Unstructured{
		"unknown repository": newRelease("apps", "a", "podinfo", "missing", "flux-system", "1.0.0"),
		"no chart name":      newRelease("apps", "b", "", "podinfo", "flux-system", "1.0.0"),
		"no source name":     newRelease("apps", "c", "podinfo", "", "flux-system", "1.0.0"),
	}
	for name, release := range cases {
		t.Run(name, func(t *testing.T) {
			client := latestClient(newRepo("podinfo", "https://example.test/charts", ""), release)
			index := &stubCharts{versions: map[string]string{"https://example.test/charts|podinfo": "9.9.9"}}

			dash := Build(context.Background(), client, latestDescs(), index)

			if releaseRow(t, dash, release.GetName()).Latest != "" {
				t.Fatalf("expected no latest version for %s", name)
			}
		})
	}
}

func TestBuildSkipsANonHelmRepositorySource(t *testing.T) {
	release := newRelease("apps", "chart-from-git", "podinfo", "flux-system", "flux-system", "1.0.0")
	if err := unstructured.SetNestedField(release.Object, "GitRepository", "spec", "chart", "spec", "sourceRef", "kind"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	client := latestClient(newRepo("flux-system", "https://example.test/charts", ""), release)
	index := &stubCharts{versions: map[string]string{"https://example.test/charts|podinfo": "9.9.9"}}

	dash := Build(context.Background(), client, latestDescs(), index)

	if releaseRow(t, dash, "chart-from-git").Latest != "" {
		t.Fatalf("a git chart source must not get a latest version")
	}
}

func TestBuildSkipsARepositoryWithoutAURL(t *testing.T) {
	client := latestClient(
		newRepo("podinfo", "", ""),
		newRelease("apps", "podinfo", "podinfo", "podinfo", "flux-system", "6.14.0"),
	)
	index := &stubCharts{versions: map[string]string{"|podinfo": "6.15.1"}}

	if releaseRow(t, Build(context.Background(), client, latestDescs(), index), "podinfo").Latest != "" {
		t.Fatalf("a repository without a url must be skipped")
	}
}

func TestBuildWithoutAChartIndex(t *testing.T) {
	client := latestClient(
		newRepo("podinfo", "https://example.test/charts", ""),
		newRelease("apps", "podinfo", "podinfo", "podinfo", "flux-system", "6.14.0"),
	)

	row := releaseRow(t, Build(context.Background(), client, latestDescs(), nil), "podinfo")

	if row.Latest != "" {
		t.Fatalf("latest = %q, want empty when no index is configured", row.Latest)
	}
}
