package gitops

import (
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

var detailKinds = map[schema.GroupVersionResource]string{
	{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:             "ApplicationList",
	{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}: "KustomizationList",
	{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}:   "GitRepositoryList",
	{Group: "apps", Version: "v1", Resource: "deployments"}:                           "DeploymentList",
	{Group: "", Version: "v1", Resource: "services"}:                                  "ServiceList",
	{Group: "", Version: "v1", Resource: "events"}:                                    "EventList",
}

func detailDescs() map[string]api.ResourceDescriptor {
	out := map[string]api.ResourceDescriptor{}
	for _, one := range []api.ResourceDescriptor{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications", Kind: "Application", Namespaced: true},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations", Kind: "Kustomization", Namespaced: true},
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories", Kind: "GitRepository", Namespaced: true},
		{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true},
		{Group: "", Version: "v1", Resource: "services", Kind: "Service", Namespaced: true},
		{Group: "", Version: "v1", Resource: "events", Kind: "Event", Namespaced: true},
	} {
		out[discovery.Key(one.Group, one.Version, one.Resource)] = one
	}
	return out
}

func detailClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), detailKinds, objs...)
}

func applicationRef() api.ObjectRef {
	return api.ObjectRef{
		Group:     "argoproj.io",
		Version:   "v1alpha1",
		Resource:  "applications",
		Namespace: "argocd",
		Name:      "podinfo",
	}
}

func managingApplication(resources ...any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]any{"name": "podinfo", "namespace": "argocd"},
		"spec": map[string]any{
			"project": "default",
			"source":  map[string]any{"repoURL": "https://example.test/apps", "path": "podinfo"},
		},
		"status": map[string]any{
			"sync":      map[string]any{"status": "Synced"},
			"health":    map[string]any{"status": "Healthy"},
			"resources": resources,
		},
	}}
}

func managed(kind, name string) map[string]any {
	entry := map[string]any{"version": "v1", "kind": kind, "name": name, "namespace": "web", "status": "Synced"}
	if kind == "Deployment" {
		entry["group"] = "apps"
	}
	return entry
}

func liveDeployment(declared string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":        "podinfo",
			"namespace":   "web",
			"annotations": map[string]any{lastAppliedAnnotation: declared},
		},
		"spec": map[string]any{"replicas": int64(3)},
	}}
}

func TestDetailRefusesAKindTheClusterDoesNotServe(t *testing.T) {
	_, err := Detail(t.Context(), detailClient(), map[string]api.ResourceDescriptor{}, applicationRef())

	if !errors.Is(err, ErrNotAnApplier) {
		t.Fatalf("error = %v, want a refusal", err)
	}
}

func TestDetailRefusesAnObjectNoControllerApplies(t *testing.T) {
	ref := api.ObjectRef{Group: "apps", Version: "v1", Resource: "deployments", Namespace: "web", Name: "podinfo"}
	client := detailClient(liveDeployment(""))

	_, err := Detail(t.Context(), client, detailDescs(), ref)

	if !errors.Is(err, ErrNotAnApplier) {
		t.Fatalf("error = %v, want a refusal", err)
	}
}

func TestDetailReportsAnApplicationThatIsNotThere(t *testing.T) {
	_, err := Detail(t.Context(), detailClient(), detailDescs(), applicationRef())

	if err == nil {
		t.Fatal("expected an error for an application that is not there")
	}
	if errors.Is(err, ErrNotAnApplier) {
		t.Fatalf("error = %v, want what the api server said", err)
	}
}

func TestDetailCarriesTheRefItWasAsked(t *testing.T) {
	client := detailClient(managingApplication())

	got, err := Detail(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if got.Ref != applicationRef() {
		t.Fatalf("ref = %+v", got.Ref)
	}
}

func TestDetailReadsPerFieldDriftFromTheLiveObject(t *testing.T) {
	client := detailClient(
		managingApplication(managed("Deployment", "podinfo")),
		liveDeployment(`{"spec":{"replicas":1}}`),
	)

	got, err := Detail(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if len(got.Resources) != 1 {
		t.Fatalf("resources = %+v", got.Resources)
	}
	drift := got.Resources[0].Drift
	if len(drift) != 1 || drift[0].Path != "spec.replicas" {
		t.Fatalf("drift = %+v, want spec.replicas", drift)
	}
	if drift[0].Declared != "1" || drift[0].Live != "3" {
		t.Fatalf("drift = %+v, want 1 -> 3", drift[0])
	}
}

func TestDetailSaysWhyItCannotComputeDrift(t *testing.T) {
	client := detailClient(managingApplication(managed("Deployment", "podinfo")), liveDeployment(""))

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if !strings.Contains(got.Resources[0].DriftNote, "no last-applied-configuration") {
		t.Fatalf("note = %q", got.Resources[0].DriftNote)
	}
}

func TestDetailExplainsAnOutOfSyncResourceWithNoFieldDrift(t *testing.T) {
	app := managingApplication(managed("Deployment", "podinfo"))
	_ = unstructured.SetNestedSlice(app.Object, []any{
		map[string]any{"group": "apps", "version": "v1", "kind": "Deployment", "name": "podinfo", "namespace": "web", "status": "OutOfSync"},
	}, "status", "resources")
	client := detailClient(app, liveDeployment(`{"spec":{"replicas":3}}`))

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if !strings.Contains(got.Resources[0].DriftNote, "git may no longer declare") {
		t.Fatalf("note = %q, want it to explain the difference", got.Resources[0].DriftNote)
	}
}

func TestDetailMarksAResourceThatIsBeingDeleted(t *testing.T) {
	live := liveDeployment(`{"spec":{"replicas":3}}`)
	now := metav1.Now()
	live.SetDeletionTimestamp(&now)
	live.SetFinalizers([]string{"foregroundDeletion"})
	client := detailClient(managingApplication(managed("Deployment", "podinfo")), live)

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if !got.Resources[0].Terminating {
		t.Fatal("a resource with a deletion timestamp is not marked as terminating")
	}
	if len(got.Resources[0].Finalizers) != 1 {
		t.Fatalf("finalizers = %v, want the one holding it", got.Resources[0].Finalizers)
	}
}

func TestDetailLeavesAResourceAloneWhenItCannotBeRead(t *testing.T) {
	client := detailClient(managingApplication(managed("Deployment", "gone")))

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if got.Resources[0].Drift != nil || got.Resources[0].DriftNote != "" {
		t.Fatalf("resource = %+v, want nothing invented", got.Resources[0])
	}
}

func TestDetailLeavesAResourceAloneWhenItsKindIsUnknown(t *testing.T) {
	client := detailClient(managingApplication(map[string]any{
		"group": "cert-manager.io", "version": "v1", "kind": "Certificate", "name": "tls", "namespace": "web",
	}))

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if got.Resources[0].DriftNote != "" {
		t.Fatalf("note = %q, want nothing for a kind we cannot look up", got.Resources[0].DriftNote)
	}
}

func TestDetailAttachesTheRecentEventsOfEachResource(t *testing.T) {
	events := make([]runtime.Object, 0, 7)
	for at := range 7 {
		events = append(events, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion":     "v1",
			"kind":           "Event",
			"metadata":       map[string]any{"name": "e" + string(rune('a'+at)), "namespace": "web"},
			"involvedObject": map[string]any{"kind": "Deployment", "name": "podinfo", "namespace": "web"},
			"type":           "Normal",
			"reason":         "Scaled",
			"message":        "scaled",
			"lastTimestamp":  "2026-08-0" + string(rune('1'+at%7)) + "T10:00:00Z",
		}})
	}
	objs := append([]runtime.Object{
		managingApplication(managed("Deployment", "podinfo")),
		liveDeployment(`{"spec":{"replicas":3}}`),
	}, events...)
	client := detailClient(objs...)

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	attached := got.Resources[0].Events
	if len(attached) != maxEventsPer {
		t.Fatalf("events = %d, want at most %d", len(attached), maxEventsPer)
	}
	if attached[0].LastSeen < attached[1].LastSeen {
		t.Fatalf("events = %+v, want the newest first", attached)
	}
}

func TestDetailOrdersEventsAtTheSameMomentDeterministically(t *testing.T) {
	events := []runtime.Object{
		gitopsEvent("third", "Progressing", "controller-z", "working"),
		gitopsEvent("second", "Healthy", "controller-z", "ready"),
		gitopsEvent("first", "Healthy", "controller-a", "ready"),
	}
	objects := append([]runtime.Object{
		managingApplication(managed("Deployment", "podinfo")),
		liveDeployment(`{"spec":{"replicas":3}}`),
	}, events...)

	got, err := Detail(t.Context(), detailClient(objects...), detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	attached := got.Resources[0].Events
	if len(attached) != 3 {
		t.Fatalf("events = %+v, want three", attached)
	}
	if attached[0].Reason != "Healthy" || attached[0].Source != "controller-a" {
		t.Fatalf("first event = %+v, want Healthy from controller-a", attached[0])
	}
	if attached[1].Reason != "Healthy" || attached[1].Source != "controller-z" {
		t.Fatalf("second event = %+v, want Healthy from controller-z", attached[1])
	}
	if attached[2].Reason != "Progressing" {
		t.Fatalf("third event = %+v, want Progressing", attached[2])
	}
}

func gitopsEvent(name, reason, source, message string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion":     "v1",
		"kind":           "Event",
		"metadata":       map[string]any{"name": name, "namespace": "web"},
		"involvedObject": map[string]any{"kind": "Deployment", "name": "podinfo", "namespace": "web"},
		"type":           "Normal",
		"reason":         reason,
		"message":        message,
		"source":         map[string]any{"component": source},
		"lastTimestamp":  "2026-08-01T10:00:00Z",
	}}
}

func TestDetailAttachesNoEventsToAResourceThatHasNone(t *testing.T) {
	client := detailClient(managingApplication(managed("Service", "podinfo")))

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if got.Resources[0].Events != nil {
		t.Fatalf("events = %+v, want none", got.Resources[0].Events)
	}
}

func TestDetailSaysHowManyResourcesItReadLive(t *testing.T) {
	entries := make([]any, 0, maxLiveReads+3)
	for at := range maxLiveReads + 3 {
		entries = append(entries, managed("Service", "svc"+string(rune('a'+at))))
	}
	client := detailClient(managingApplication(entries...))

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	found := false
	for _, one := range got.Issues {
		if strings.Contains(one.Title, "were read from the cluster") {
			found = true
			if one.Severity != api.SeverityInfo {
				t.Fatalf("severity = %q, want info", one.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("issues = %+v, want the cap stated", got.Issues)
	}
}

func TestDetailReadsTheOutOfSyncResourcesFirst(t *testing.T) {
	entries := make([]any, 0, maxLiveReads+1)
	for at := range maxLiveReads {
		entries = append(entries, managed("Service", "svc"+string(rune('a'+at))))
	}
	entries = append(entries, map[string]any{
		"group": "apps", "version": "v1", "kind": "Deployment", "name": "podinfo",
		"namespace": "web", "status": "OutOfSync",
	})
	client := detailClient(managingApplication(entries...), liveDeployment(`{"spec":{"replicas":1}}`))

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	last := got.Resources[len(got.Resources)-1]
	if len(last.Drift) != 1 {
		t.Fatalf("the out of sync resource was not read live: %+v", last)
	}
}

func TestFluxDetailResolvesTheRepositoryURL(t *testing.T) {
	kustomization := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": "apps", "namespace": "flux-system"},
		"spec": map[string]any{
			"path":      "./apps",
			"sourceRef": map[string]any{"kind": "GitRepository", "name": "flux-system"},
		},
	}}
	source := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "GitRepository",
		"metadata":   map[string]any{"name": "flux-system", "namespace": "flux-system"},
		"spec": map[string]any{
			"url": "https://github.com/example/infra",
			"ref": map[string]any{"branch": "main"},
		},
	}}
	client := detailClient(kustomization, source)
	ref := api.ObjectRef{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
		Namespace: "flux-system", Name: "apps",
	}

	got, err := Detail(t.Context(), client, detailDescs(), ref)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if got.Source.Repo != "https://github.com/example/infra" {
		t.Fatalf("repo = %q, want the url from the source object", got.Source.Repo)
	}
	if got.Source.Target != "main" {
		t.Fatalf("target = %q, want the branch", got.Source.Target)
	}
}

func TestFluxDetailKeepsTheRefWhenTheSourceIsNotThere(t *testing.T) {
	kustomization := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": "apps", "namespace": "flux-system"},
		"spec": map[string]any{
			"sourceRef": map[string]any{"kind": "GitRepository", "name": "missing"},
		},
	}}
	client := detailClient(kustomization)
	ref := api.ObjectRef{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
		Namespace: "flux-system", Name: "apps",
	}

	got, _ := Detail(t.Context(), client, detailDescs(), ref)

	if got.Source.Repo != "GitRepository/missing" {
		t.Fatalf("repo = %q, want the ref left as it was", got.Source.Repo)
	}
}

func TestFluxDetailKeepsTheRefWhenTheSourceKindIsUnknown(t *testing.T) {
	kustomization := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": "apps", "namespace": "flux-system"},
		"spec": map[string]any{
			"sourceRef": map[string]any{"kind": "Bucket", "name": "artifacts"},
		},
	}}
	client := detailClient(kustomization)
	ref := api.ObjectRef{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
		Namespace: "flux-system", Name: "apps",
	}

	got, _ := Detail(t.Context(), client, detailDescs(), ref)

	if got.Source.Repo != "Bucket/artifacts" {
		t.Fatalf("repo = %q", got.Source.Repo)
	}
}

func TestFluxDetailLeavesTheRepoAloneWhenThereIsNoSourceAtAll(t *testing.T) {
	kustomization := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": "apps", "namespace": "flux-system"},
		"spec":       map[string]any{"path": "./apps"},
	}}
	client := detailClient(kustomization)
	ref := api.ObjectRef{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
		Namespace: "flux-system", Name: "apps",
	}

	got, _ := Detail(t.Context(), client, detailDescs(), ref)

	if got.Source.Repo != "" {
		t.Fatalf("repo = %q, want nothing", got.Source.Repo)
	}
}

func TestFluxDetailReadsATagWhenThereIsNoBranch(t *testing.T) {
	source := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "GitRepository",
		"metadata":   map[string]any{"name": "flux-system", "namespace": "flux-system"},
		"spec": map[string]any{
			"url": "https://github.com/example/infra",
			"ref": map[string]any{"tag": "v1.2.3"},
		},
	}}
	kustomization := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": "apps", "namespace": "flux-system"},
		"spec":       map[string]any{"sourceRef": map[string]any{"kind": "GitRepository", "name": "flux-system"}},
	}}
	client := detailClient(kustomization, source)
	ref := api.ObjectRef{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
		Namespace: "flux-system", Name: "apps",
	}

	got, _ := Detail(t.Context(), client, detailDescs(), ref)

	if got.Source.Target != "v1.2.3" {
		t.Fatalf("target = %q, want the tag", got.Source.Target)
	}
}

func TestDetailNamesThePluralResourceSoTheUICanOpenIt(t *testing.T) {
	client := detailClient(managingApplication(managed("Deployment", "podinfo")))

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if got.Resources[0].Resource != "deployments" {
		t.Fatalf("resource = %q, want deployments", got.Resources[0].Resource)
	}
}

func TestDetailKeepsTheVersionTheControllerReported(t *testing.T) {
	client := detailClient(managingApplication(map[string]any{
		"group": "apps", "version": "v1beta1", "kind": "Deployment", "name": "podinfo", "namespace": "web",
	}))

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if got.Resources[0].Version != "v1beta1" {
		t.Fatalf("version = %q, want the one the controller reported", got.Resources[0].Version)
	}
}

func TestABrokenResourceIsReadBeforeADriftedOne(t *testing.T) {
	resources := []api.GitopsResource{
		{Kind: "Service", Name: "drifted", Sync: "OutOfSync", Health: "Healthy"},
		{Kind: "Service", Name: "broken", Sync: "Synced", Health: "Degraded"},
	}

	order := readingOrder(resources)

	if resources[order[0]].Name != "broken" {
		t.Fatalf("first read = %q, want the degraded one before the drifted one", resources[order[0]].Name)
	}
}

func TestAResourceThatIsBothBrokenAndDriftedGoesFirst(t *testing.T) {
	resources := []api.GitopsResource{
		{Kind: "Service", Name: "drifted", Sync: "OutOfSync", Health: "Healthy"},
		{Kind: "Service", Name: "both", Sync: "OutOfSync", Health: "Degraded"},
		{Kind: "Service", Name: "fine", Sync: "Synced", Health: "Healthy"},
	}

	order := readingOrder(resources)

	if resources[order[0]].Name != "both" {
		t.Fatalf("first read = %q, want the one that is both", resources[order[0]].Name)
	}
	if resources[order[2]].Name != "fine" {
		t.Fatalf("last read = %q, want the healthy one last", resources[order[2]].Name)
	}
}

func TestResourcesTheControllerSaysNothingAboutKeepTheirOrder(t *testing.T) {
	resources := []api.GitopsResource{
		{Kind: "Service", Name: "first"},
		{Kind: "Service", Name: "second"},
	}

	order := readingOrder(resources)

	if resources[order[0]].Name != "first" || resources[order[1]].Name != "second" {
		t.Fatalf("order = %v, want the list order kept when nothing distinguishes them", order)
	}
}

func TestEventsAreAskedForByTheObjectTheyBelongTo(t *testing.T) {
	client := detailClient(
		managingApplication(managed("Deployment", "podinfo")),
		liveDeployment(`{"spec":{"replicas":3}}`),
	)
	seen := ""
	client.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		listing, ok := action.(k8stesting.ListAction)
		if ok {
			seen = listing.GetListRestrictions().Fields.String()
		}
		return false, nil, nil
	})

	_, err := Detail(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	want := "involvedObject.kind=Deployment,involvedObject.name=podinfo"
	if seen != want {
		t.Fatalf("field selector = %q, want %q so a busy namespace cannot hide them", seen, want)
	}
}

func TestEventsAreNotAskedForAResourceThatWasNotReadLive(t *testing.T) {
	entries := make([]any, 0, maxLiveReads+1)
	for at := range maxLiveReads + 1 {
		entries = append(entries, managed("Service", "svc"+string(rune('a'+at))))
	}
	client := detailClient(managingApplication(entries...))
	lists := 0
	client.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		lists++
		return false, nil, nil
	})

	_, err := Detail(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if lists != maxLiveReads {
		t.Fatalf("event lists = %d, want one per resource read live, not %d", lists, maxLiveReads+1)
	}
}

func TestAResourceWithNoNamespaceIsNotAskedForEvents(t *testing.T) {
	client := detailClient(managingApplication(map[string]any{
		"version": "v1", "kind": "Service", "name": "cluster-wide",
	}))
	lists := 0
	client.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		lists++
		return false, nil, nil
	})

	_, err := Detail(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if lists != 0 {
		t.Fatalf("event lists = %d, want none for a resource with no namespace", lists)
	}
}

func TestEventsSurviveANamespaceThatRefusesToListThem(t *testing.T) {
	client := detailClient(
		managingApplication(managed("Deployment", "podinfo")),
		liveDeployment(`{"spec":{"replicas":3}}`),
	)
	client.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("events are forbidden here")
	})

	got, err := Detail(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if got.Resources[0].Events != nil {
		t.Fatalf("events = %+v, want none rather than a failed page", got.Resources[0].Events)
	}
	if len(got.Resources[0].Drift) != 0 {
		t.Fatalf("drift = %+v, want the rest of the read to survive", got.Resources[0].Drift)
	}
}

func TestBranchOfTakesEveryRefFluxAccepts(t *testing.T) {
	cases := []struct {
		name string
		ref  map[string]any
		want string
	}{
		{name: "a branch", ref: map[string]any{"branch": "main"}, want: "main"},
		{name: "a tag", ref: map[string]any{"tag": "v1.2.3"}, want: "v1.2.3"},
		{name: "a semver range", ref: map[string]any{"semver": ">=1.0.0"}, want: ">=1.0.0"},
		{name: "a commit", ref: map[string]any{"commit": "abc1234"}, want: "abc1234"},
		{name: "an oci tag by name", ref: map[string]any{"name": "latest"}, want: "latest"},
		{
			name: "a branch wins over a commit beside it",
			ref:  map[string]any{"branch": "main", "commit": "abc1234"},
			want: "main",
		},
		{name: "no ref at all", ref: map[string]any{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "source.toolkit.fluxcd.io/v1",
				"kind":       "GitRepository",
				"metadata":   map[string]any{"name": "infra", "namespace": "flux-system"},
				"spec":       map[string]any{"ref": tc.ref},
			}}

			if got := branchOf(source); got != tc.want {
				t.Fatalf("branchOf(%v) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestIdentifyLeavesAResourceWhoseKindTheClusterDoesNotServe(t *testing.T) {
	resources := []api.GitopsResource{
		{Group: "cert-manager.io", Kind: "Certificate", Name: "tls", Version: "v1"},
	}

	identify(detailDescs(), resources)

	if resources[0].Resource != "" {
		t.Fatalf("resource = %q, want nothing for a kind the cluster does not serve", resources[0].Resource)
	}
	if resources[0].Version != "v1" {
		t.Fatalf("version = %q, want the one the controller reported", resources[0].Version)
	}
}

func TestIdentifyFillsTheVersionOnlyWhenTheControllerLeftItOut(t *testing.T) {
	resources := []api.GitopsResource{
		{Group: "apps", Kind: "Deployment", Name: "web"},
		{Group: "apps", Kind: "Deployment", Name: "api", Version: "v1beta1"},
	}

	identify(detailDescs(), resources)

	if resources[0].Version != "v1" {
		t.Fatalf("version = %q, want discovery to fill it in", resources[0].Version)
	}
	if resources[1].Version != "v1beta1" {
		t.Fatalf("version = %q, want the controller's own left alone", resources[1].Version)
	}
}

func ssaDeployment(managers ...string) *unstructured.Unstructured {
	live := liveDeployment("")
	live.SetAnnotations(nil)
	fields := map[string]string{
		"argocd-controller":    `{"f:spec":{"f:selector":{}}}`,
		"kustomize-controller": `{"f:spec":{"f:selector":{}}}`,
		"kubectl-edit":         `{"f:spec":{"f:replicas":{}}}`,
	}
	entries := make([]metav1.ManagedFieldsEntry, 0, len(managers))
	for _, manager := range managers {
		entries = append(entries, metav1.ManagedFieldsEntry{
			Manager:    manager,
			Operation:  metav1.ManagedFieldsOperationApply,
			FieldsType: "FieldsV1",
			FieldsV1:   &metav1.FieldsV1{Raw: []byte(fields[manager])},
		})
	}
	live.SetManagedFields(entries)
	return live
}

func TestAServerSideAppliedResourceNamesWhoTookTheField(t *testing.T) {
	client := detailClient(
		managingApplication(managed("Deployment", "podinfo")),
		ssaDeployment("argocd-controller", "kubectl-edit"),
	)

	got, err := Detail(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	drift := got.Resources[0].Drift
	if len(drift) != 1 || drift[0].Path != "spec.replicas" {
		t.Fatalf("drift = %+v, want the field another writer holds", drift)
	}
	if drift[0].Declared != "argocd-controller" || drift[0].Live != "kubectl-edit" {
		t.Fatalf("drift = %+v, want the two managers named", drift[0])
	}
	if !strings.Contains(got.Resources[0].DriftNote, "applied server-side") {
		t.Fatalf("note = %q, want it to say these are owners, not values", got.Resources[0].DriftNote)
	}
}

func TestAServerSideAppliedResourceNobodyTookSaysSo(t *testing.T) {
	client := detailClient(
		managingApplication(managed("Deployment", "podinfo")),
		ssaDeployment("argocd-controller"),
	)

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if len(got.Resources[0].Drift) != 0 {
		t.Fatalf("drift = %+v, want none", got.Resources[0].Drift)
	}
	if !strings.Contains(got.Resources[0].DriftNote, "argocd-controller") {
		t.Fatalf("note = %q, want the controller named as still holding its fields", got.Resources[0].DriftNote)
	}
}

func TestAnObjectWithNeitherADeclarationNorAGitopsManagerStillSaysWhy(t *testing.T) {
	client := detailClient(
		managingApplication(managed("Deployment", "podinfo")),
		ssaDeployment("kubectl-edit"),
	)

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	if !strings.Contains(got.Resources[0].DriftNote, "no last-applied-configuration") {
		t.Fatalf("note = %q, want the original explanation", got.Resources[0].DriftNote)
	}
}

func TestADeclarationStillWinsOverOwnership(t *testing.T) {
	live := liveDeployment(`{"spec":{"replicas":1}}`)
	live.SetManagedFields([]metav1.ManagedFieldsEntry{{
		Manager:    "argocd-controller",
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsType: "FieldsV1",
		FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:replicas":{}}}`)},
	}})
	client := detailClient(managingApplication(managed("Deployment", "podinfo")), live)

	got, _ := Detail(t.Context(), client, detailDescs(), applicationRef())

	drift := got.Resources[0].Drift
	if len(drift) != 1 || drift[0].Declared != "1" || drift[0].Live != "3" {
		t.Fatalf("drift = %+v, want the value diff, not the ownership one", drift)
	}
}

func countingClient(t *testing.T, objs ...runtime.Object) (*fake.FakeDynamicClient, func() int) {
	t.Helper()
	client := detailClient(objs...)
	listed := 0
	client.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		listed++
		return false, nil, nil
	})
	return client, func() int { return listed }
}

func TestTheGraphReadDoesNotAskForEvents(t *testing.T) {
	client, listed := countingClient(
		t,
		managingApplication(managed("Deployment", "podinfo")),
		liveDeployment(`{"spec":{"replicas":3}}`),
	)

	_, err := Shape(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("shape: %v", err)
	}

	if listed() != 0 {
		t.Fatalf("the graph read listed events %d times; the graph draws none of them", listed())
	}
}

func TestThePanelReadStillAsksForEvents(t *testing.T) {
	client, listed := countingClient(
		t,
		managingApplication(managed("Deployment", "podinfo")),
		liveDeployment(`{"spec":{"replicas":3}}`),
	)

	_, err := Detail(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if listed() == 0 {
		t.Fatal("the panel read asked for no events; the panel shows them")
	}
}

func TestTheGraphStillKnowsWhetherEachResourceIsReady(t *testing.T) {
	client := detailClient(
		managingApplication(managed("Deployment", "podinfo")),
		liveDeployment(`{"spec":{"replicas":3}}`),
	)

	shape, err := Shape(t.Context(), client, detailDescs(), applicationRef())
	if err != nil {
		t.Fatalf("shape: %v", err)
	}

	graph := AppGraph(shape)
	if len(graph.Nodes) < 2 {
		t.Fatalf("nodes = %d, want the app and what it manages", len(graph.Nodes))
	}
	for _, node := range graph.Nodes[1:] {
		if node.Ready == "" {
			t.Fatalf("%s has no readiness; skipping events must not skip the live read", node.ID)
		}
	}
}
