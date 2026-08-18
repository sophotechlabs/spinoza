package argocd

import (
	"context"
	"errors"
	"maps"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type stubLister struct {
	items  map[string][]*unstructured.Unstructured
	errs   map[string]error
	warmed []api.ResourceDescriptor
}

func (s *stubLister) List(
	_ context.Context,
	desc api.ResourceDescriptor,
) ([]*unstructured.Unstructured, error) {
	err, refused := s.errs[desc.Resource]
	if refused {
		return nil, err
	}
	return s.items[desc.Resource], nil
}

func (s *stubLister) Warm(_ context.Context, descs []api.ResourceDescriptor) {
	s.warmed = append(s.warmed, descs...)
}

func appDescriptor() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      Group,
		Version:    "v1alpha1",
		Resource:   applications,
		Kind:       "Application",
		Namespaced: true,
	}
}

func setDescriptor() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      Group,
		Version:    "v1alpha1",
		Resource:   applicationSets,
		Kind:       "ApplicationSet",
		Namespaced: true,
	}
}

func projectDescriptor() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      Group,
		Version:    "v1alpha1",
		Resource:   appProjects,
		Kind:       "AppProject",
		Namespaced: true,
	}
}

func catalog(descs ...api.ResourceDescriptor) map[string]api.ResourceDescriptor {
	out := map[string]api.ResourceDescriptor{}
	for _, desc := range descs {
		out[desc.Group+"/"+desc.Version+"/"+desc.Resource] = desc
	}
	out["apps/v1/deployments"] = api.ResourceDescriptor{
		Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment",
	}
	return out
}

func app(name string, fields map[string]any) *unstructured.Unstructured {
	object := map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]any{"name": name, "namespace": "argocd"},
	}
	maps.Copy(object, fields)
	return &unstructured.Unstructured{Object: object}
}

func healthy(name, revision string) *unstructured.Unstructured {
	return app(name, map[string]any{
		"spec": map[string]any{
			"project":     "default",
			"source":      map[string]any{"repoURL": "https://git/apps", "path": "apps/" + name},
			"destination": map[string]any{"server": "in-cluster", "namespace": name},
		},
		"status": map[string]any{
			"sync":   map[string]any{"status": "Synced", "revision": revision},
			"health": map[string]any{"status": "Healthy"},
		},
	})
}

func onlyApps(items ...*unstructured.Unstructured) *stubLister {
	return &stubLister{items: map[string][]*unstructured.Unstructured{applications: items}}
}

func build(t *testing.T, lister Lister, descs map[string]api.ResourceDescriptor) api.ArgoDashboard {
	t.Helper()
	return Build(context.Background(), lister, descs)
}

func TestABuildWithoutArgoTypesAsksForNothing(t *testing.T) {
	lister := onlyApps(healthy("web", "abc123"))

	found := build(t, lister, catalog())

	if len(found.Apps) != 0 {
		t.Fatalf("apps = %+v, want none", found.Apps)
	}
	if len(lister.warmed) != 0 {
		t.Fatalf("warmed %d types on a cluster with no argo", len(lister.warmed))
	}
}

func TestApplicationsCarryTheirSyncAndHealth(t *testing.T) {
	lister := onlyApps(healthy("web", "abc123"))

	found := build(t, lister, catalog(appDescriptor()))

	if len(found.Apps) != 1 {
		t.Fatalf("apps = %+v, want one", found.Apps)
	}
	got := found.Apps[0]
	if got.Sync != "Synced" || got.Health != "Healthy" {
		t.Fatalf("status = %s/%s", got.Sync, got.Health)
	}
	if got.Project != "default" || got.Repo != "https://git/apps" || got.Path != "apps/web" {
		t.Fatalf("app = %+v", got)
	}
	if got.Destination != "in-cluster web" {
		t.Fatalf("destination = %q", got.Destination)
	}
	if got.Revision != "abc123" {
		t.Fatalf("revision = %q", got.Revision)
	}
	if got.Kind != "Application" {
		t.Fatalf("kind = %q", got.Kind)
	}
}

func TestApplicationsComeBackInAStableOrder(t *testing.T) {
	first := healthy("web", "a")
	first.SetNamespace("team-b")
	second := healthy("api", "b")
	second.SetNamespace("team-a")
	lister := onlyApps(first, second)

	found := build(t, lister, catalog(appDescriptor()))

	if found.Apps[0].Name != "api" || found.Apps[1].Name != "web" {
		t.Fatalf("order = %s, %s", found.Apps[0].Name, found.Apps[1].Name)
	}
}

func TestAnAppOfAppsChildNamesItsParent(t *testing.T) {
	parent := healthy("root", "a")
	child := healthy("web", "b")
	child.SetLabels(map[string]string{trackingLabel: "root"})
	lister := onlyApps(parent, child)

	found := build(t, lister, catalog(appDescriptor()))

	for _, got := range found.Apps {
		if got.Name == "web" && got.Owner != "root" {
			t.Fatalf("owner = %q, want root", got.Owner)
		}
		if got.Name == "root" && got.Owner != "" {
			t.Fatalf("the parent claims an owner: %q", got.Owner)
		}
	}
}

func TestTheTrackingAnnotationAlsoNamesTheParent(t *testing.T) {
	parent := healthy("root", "a")
	child := healthy("web", "b")
	child.SetAnnotations(map[string]string{
		trackingAnnotation: "root:argoproj.io/Application:argocd/web",
	})
	lister := onlyApps(parent, child)

	found := build(t, lister, catalog(appDescriptor()))

	for _, got := range found.Apps {
		if got.Name == "web" && got.Owner != "root" {
			t.Fatalf("owner = %q, want root", got.Owner)
		}
	}
}

func TestAnAppTrackedByItselfHasNoParent(t *testing.T) {
	only := healthy("web", "a")
	only.SetLabels(map[string]string{trackingLabel: "web"})
	lister := onlyApps(only)

	found := build(t, lister, catalog(appDescriptor()))

	if found.Apps[0].Owner != "" {
		t.Fatalf("owner = %q, want none", found.Apps[0].Owner)
	}
}

func TestAParentThatIsNotAnApplicationIsDropped(t *testing.T) {
	child := healthy("web", "a")
	child.SetLabels(map[string]string{trackingLabel: "some-helm-release"})
	lister := onlyApps(child)

	found := build(t, lister, catalog(appDescriptor()))

	if found.Apps[0].Owner != "" {
		t.Fatalf("owner = %q, want none for a parent that is not an app", found.Apps[0].Owner)
	}
}

func TestAGeneratedAppNamesItsApplicationSet(t *testing.T) {
	child := healthy("web", "a")
	child.SetOwnerReferences([]metav1.OwnerReference{{Kind: "ApplicationSet", Name: "fleet"}})
	lister := &stubLister{
		items: map[string][]*unstructured.Unstructured{
			applications:    {child},
			applicationSets: {app("fleet", map[string]any{})},
		},
	}

	found := build(t, lister, catalog(appDescriptor(), setDescriptor()))

	if found.Apps[0].Owner != "fleet" {
		t.Fatalf("owner = %q, want the application set", found.Apps[0].Owner)
	}
	if len(found.ApplicationSets) != 1 {
		t.Fatalf("sets = %+v, want one", found.ApplicationSets)
	}
}

func TestAnAppWithSeveralSourcesStillNamesOne(t *testing.T) {
	many := app("web", map[string]any{
		"spec": map[string]any{
			"sources": []any{
				map[string]any{"repoURL": "https://git/values", "targetRevision": "main"},
				map[string]any{"repoURL": "https://git/chart", "chart": "podinfo"},
			},
		},
	})
	lister := onlyApps(many)

	found := build(t, lister, catalog(appDescriptor()))

	if found.Apps[0].Repo != "https://git/values" {
		t.Fatalf("repo = %q", found.Apps[0].Repo)
	}
	if found.Apps[0].Path != "podinfo" {
		t.Fatalf("path = %q, want the chart when there is no path", found.Apps[0].Path)
	}
	if found.Apps[0].Revision != "main" {
		t.Fatalf("revision = %q, want the target when nothing synced yet", found.Apps[0].Revision)
	}
}

func TestAnAppWithNoStatusIsStillListed(t *testing.T) {
	bare := app("web", map[string]any{"spec": map[string]any{"project": "default"}})
	lister := onlyApps(bare)

	found := build(t, lister, catalog(appDescriptor()))

	if len(found.Apps) != 1 {
		t.Fatalf("apps = %+v, want the app", found.Apps)
	}
	if found.Apps[0].Sync != "" || found.Apps[0].Health != "" {
		t.Fatalf("app = %+v, want empty status", found.Apps[0])
	}
}

func TestAHealthMessageIsCarried(t *testing.T) {
	sick := app("web", map[string]any{
		"status": map[string]any{
			"health": map[string]any{"status": "Degraded", "message": "pod crashlooping"},
		},
	})
	lister := onlyApps(sick)

	found := build(t, lister, catalog(appDescriptor()))

	if found.Apps[0].Message != "pod crashlooping" {
		t.Fatalf("message = %q", found.Apps[0].Message)
	}
}

func TestAnOperationMessageStandsInForHealth(t *testing.T) {
	failing := app("web", map[string]any{
		"status": map[string]any{
			"operationState": map[string]any{"message": "one or more objects failed"},
		},
	})
	lister := onlyApps(failing)

	found := build(t, lister, catalog(appDescriptor()))

	if found.Apps[0].Message != "one or more objects failed" {
		t.Fatalf("message = %q", found.Apps[0].Message)
	}
}

func TestADestinationWithoutANamespaceIsJustTheServer(t *testing.T) {
	remote := app("web", map[string]any{
		"spec": map[string]any{"destination": map[string]any{"server": "https://kube"}},
	})
	lister := onlyApps(remote)

	found := build(t, lister, catalog(appDescriptor()))

	if found.Apps[0].Destination != "https://kube" {
		t.Fatalf("destination = %q", found.Apps[0].Destination)
	}
}

func TestADestinationWithoutAServerIsJustTheNamespace(t *testing.T) {
	local := app("web", map[string]any{
		"spec": map[string]any{"destination": map[string]any{"namespace": "shop"}},
	})
	lister := onlyApps(local)

	found := build(t, lister, catalog(appDescriptor()))

	if found.Apps[0].Destination != "shop" {
		t.Fatalf("destination = %q", found.Apps[0].Destination)
	}
}

func TestAKindThatCannotBeListedIsReported(t *testing.T) {
	lister := &stubLister{errs: map[string]error{applications: errors.New("applications is forbidden")}}

	found := build(t, lister, catalog(appDescriptor()))

	if found.Error == "" {
		t.Fatal("a refusal was swallowed")
	}
	if len(found.Apps) != 0 {
		t.Fatalf("apps = %+v, want none", found.Apps)
	}
}

func TestApplicationsStillArriveWhenTheSetsAreRefused(t *testing.T) {
	lister := &stubLister{
		items: map[string][]*unstructured.Unstructured{applications: {healthy("web", "a")}},
		errs:  map[string]error{applicationSets: errors.New("applicationsets is forbidden")},
	}

	found := build(t, lister, catalog(appDescriptor(), setDescriptor()))

	if len(found.Apps) != 1 {
		t.Fatalf("apps = %+v, want the application", found.Apps)
	}
	if found.Error == "" {
		t.Fatal("the refusal was swallowed")
	}
}

func TestProjectsComeBackOnTheirOwn(t *testing.T) {
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		applications: {healthy("web", "a")},
		appProjects:  {app("default", nil), app("infra", nil)},
	}}

	found := build(t, lister, catalog(appDescriptor(), projectDescriptor()))

	if len(found.Projects) != 2 {
		t.Fatalf("projects = %+v, want two", found.Projects)
	}
	if found.Projects[0].Name != "default" || found.Projects[1].Name != "infra" {
		t.Fatalf("projects came back out of order: %+v", found.Projects)
	}
	if found.Projects[0].Kind != "AppProject" {
		t.Fatalf("kind = %q", found.Projects[0].Kind)
	}
	if len(found.Apps) != 1 {
		t.Fatalf("apps = %+v, want the application", found.Apps)
	}
}

func TestProjectsAreNotParentsOfApplications(t *testing.T) {
	child := healthy("web", "a")
	child.SetLabels(map[string]string{trackingLabel: "default"})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		applications: {child},
		appProjects:  {app("default", nil)},
	}}

	found := build(t, lister, catalog(appDescriptor(), projectDescriptor()))

	if found.Apps[0].Owner != "" {
		t.Fatalf("owner = %q, want the project ignored", found.Apps[0].Owner)
	}
}

func TestOnlyArgoTypesAreWarmed(t *testing.T) {
	lister := onlyApps(healthy("web", "a"))

	build(t, lister, catalog(appDescriptor(), setDescriptor()))

	for _, desc := range lister.warmed {
		if desc.Group != Group {
			t.Fatalf("warmed %s, which is not argo", desc.Resource)
		}
	}
	if len(lister.warmed) != 2 {
		t.Fatalf("warmed %d types, want applications and applicationsets", len(lister.warmed))
	}
}

func TestInstalledFollowsTheApplicationType(t *testing.T) {
	if Installed(catalog()) {
		t.Fatal("argo was reported on a cluster without it")
	}
	if !Installed(catalog(appDescriptor())) {
		t.Fatal("argo was not reported on a cluster with applications")
	}
}
