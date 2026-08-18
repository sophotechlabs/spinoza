package flux

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

func controllerDeployment(name, version string, ready int32) *appsv1.Deployment {
	one := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "flux-system",
			Labels: map[string]string{
				"app.kubernetes.io/part-of":   "flux",
				"app.kubernetes.io/component": name,
				"app.kubernetes.io/version":   version,
			},
		},
		Spec:   appsv1.DeploymentSpec{Replicas: &one},
		Status: appsv1.DeploymentStatus{ReadyReplicas: ready},
	}
}

func operatorDeployment(version string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "flux-operator",
			Namespace: "flux-system",
			Labels: map[string]string{
				"app.kubernetes.io/name":    "flux-operator",
				"app.kubernetes.io/version": version,
			},
		},
	}
}

func controllerPod(name, cpu, memory string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "flux-system",
			Labels:    map[string]string{"app.kubernetes.io/part-of": "flux"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "manager",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(cpu),
					corev1.ResourceMemory: resource.MustParse(memory),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		}}},
	}
}

func overviewDescs() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		discovery.Key("kustomize.toolkit.fluxcd.io", "v1", "kustomizations"): {
			Group: "kustomize.toolkit.fluxcd.io", Version: "v1",
			Resource: "kustomizations", Kind: "Kustomization", Namespaced: true,
		},
		discovery.Key("source.toolkit.fluxcd.io", "v1", "gitrepositories"): {
			Group: "source.toolkit.fluxcd.io", Version: "v1",
			Resource: "gitrepositories", Kind: "GitRepository", Namespaced: true,
		},
		discovery.Key("fluxcd.controlplane.io", "v1", "fluxinstances"): {
			Group: "fluxcd.controlplane.io", Version: "v1",
			Resource: "fluxinstances", Kind: "FluxInstance", Namespaced: true,
		},
	}
}

func overviewKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}: "KustomizationList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}:   "GitRepositoryList",
		{Group: "fluxcd.controlplane.io", Version: "v1", Resource: "fluxinstances"}:       "FluxInstanceList",
	}
}

func syncKustomization() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": "flux-system", "namespace": "flux-system"},
		"spec": map[string]any{
			"path":      "./clusters/p-mk2",
			"sourceRef": map[string]any{"kind": "GitRepository", "name": "flux-system"},
		},
		"status": map[string]any{
			"lastAppliedRevision": "refs/heads/main@sha1:abc",
			"conditions":          []any{map[string]any{"type": "Ready", "status": "True"}},
		},
	}}
}

func syncSourceRepo() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "source.toolkit.fluxcd.io/v1",
		"kind":       "GitRepository",
		"metadata":   map[string]any{"name": "flux-system", "namespace": "flux-system"},
		"spec": map[string]any{
			"url": "ssh://git@github.com/sophotechlabs/hetzner-gitops",
			"ref": map[string]any{"name": "refs/heads/main"},
		},
	}}
}

func fluxInstanceCR() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "fluxcd.controlplane.io/v1",
		"kind":       "FluxInstance",
		"metadata":   map[string]any{"name": "flux", "namespace": "flux-system"},
		"spec":       map[string]any{"distribution": map[string]any{"version": "2.9.4"}},
	}}
}

func overviewFor(
	t *testing.T,
	objects []runtime.Object,
	crs []runtime.Object,
	usage map[string]api.ResourceUsage,
) api.FluxOverview {
	t.Helper()
	cs := k8sfake.NewClientset(objects...)
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), overviewKinds(), crs...,
	)
	return Overview(context.Background(), cs, listerFor(dyn), overviewDescs(), Cluster{
		Kubernetes: "v1.36.2+k3s1",
		Nodes:      1,
		Usage:      usage,
	})
}

func TestOverviewReadsTheControllersAndTheirVersion(t *testing.T) {
	got := overviewFor(t, []runtime.Object{
		controllerDeployment("source-controller", "v2.9.4", 1),
		controllerDeployment("kustomize-controller", "v2.9.4", 1),
	}, nil, nil)

	if len(got.Controllers) != 2 {
		t.Fatalf("controllers = %+v, want two", got.Controllers)
	}
	if got.Distribution != "v2.9.4" {
		t.Fatalf("distribution = %q, want the shared controller version", got.Distribution)
	}
	if got.Namespace != "flux-system" {
		t.Fatalf("namespace = %q", got.Namespace)
	}
	if !got.Ready {
		t.Fatalf("summary = %q, want a ready cluster", got.Summary)
	}
}

func TestOverviewNamesAControllerThatIsShort(t *testing.T) {
	got := overviewFor(t, []runtime.Object{
		controllerDeployment("source-controller", "v2.9.4", 1),
		controllerDeployment("helm-controller", "v2.9.4", 0),
	}, nil, nil)

	if got.Ready {
		t.Fatal("a cluster with a controller down reported ready")
	}
	if got.Summary != "helm-controller is not ready" {
		t.Fatalf("summary = %q", got.Summary)
	}
}

func TestOverviewLeavesTheOperatorOutWhenItIsNotInstalled(t *testing.T) {
	got := overviewFor(t, []runtime.Object{
		controllerDeployment("source-controller", "v2.9.4", 1),
	}, nil, nil)

	if got.Operator != "" {
		t.Fatalf("operator = %q, want nothing on plain flux", got.Operator)
	}
	if got.Distribution != "v2.9.4" {
		t.Fatalf("distribution = %q, want the version the controllers carry", got.Distribution)
	}
}

func TestOverviewReportsTheOperatorWhenItIsThere(t *testing.T) {
	got := overviewFor(t, []runtime.Object{
		controllerDeployment("source-controller", "v2.9.4", 1),
		operatorDeployment("v0.58.0"),
	}, []runtime.Object{fluxInstanceCR()}, nil)

	if got.Operator != "v0.58.0" {
		t.Fatalf("operator = %q, want v0.58.0", got.Operator)
	}
	if got.Distribution != "2.9.4" {
		t.Fatalf("distribution = %q, want what the instance asks for", got.Distribution)
	}
}

func TestOverviewFollowsTheSyncToItsSource(t *testing.T) {
	got := overviewFor(
		t,
		[]runtime.Object{controllerDeployment("kustomize-controller", "v2.9.4", 1)},
		[]runtime.Object{syncKustomization(), syncSourceRepo()},
		nil,
	)

	if got.Sync.Kind != "Kustomization" || got.Sync.Name != "flux-system" {
		t.Fatalf("sync = %+v", got.Sync)
	}
	if got.Sync.URL != "ssh://git@github.com/sophotechlabs/hetzner-gitops" {
		t.Fatalf("url = %q", got.Sync.URL)
	}
	if got.Sync.Ref != "refs/heads/main" {
		t.Fatalf("ref = %q", got.Sync.Ref)
	}
	if got.Sync.Path != "./clusters/p-mk2" {
		t.Fatalf("path = %q", got.Sync.Path)
	}
	if got.Sync.Revision != "refs/heads/main@sha1:abc" {
		t.Fatalf("revision = %q", got.Sync.Revision)
	}
	if !got.Sync.Ready {
		t.Fatal("a Ready=True sync reported not ready")
	}
}

func TestOverviewFindsTheSyncTheOperatorCreated(t *testing.T) {
	got := overviewFor(
		t,
		[]runtime.Object{
			controllerDeployment("kustomize-controller", "v2.9.4", 1),
			operatorDeployment("v0.58.0"),
		},
		[]runtime.Object{fluxInstanceCR(), syncKustomization(), syncSourceRepo()},
		nil,
	)

	if got.Sync.Name != "flux-system" {
		t.Fatalf("sync = %+v, want the flux-system kustomization, not the instance name", got.Sync)
	}
	if got.Sync.Revision != "refs/heads/main@sha1:abc" {
		t.Fatalf("revision = %q", got.Sync.Revision)
	}
}

func TestOverviewFollowsTheSyncNameTheInstanceAsksFor(t *testing.T) {
	instance := fluxInstanceCR()
	spec, ok := instance.Object["spec"].(map[string]any)
	if !ok {
		t.Fatal("the instance fixture has no spec")
	}
	spec["sync"] = map[string]any{"name": "elsewhere"}

	got := overviewFor(
		t,
		[]runtime.Object{controllerDeployment("kustomize-controller", "v2.9.4", 1)},
		[]runtime.Object{instance, syncKustomization()},
		nil,
	)

	if got.Sync.Name != "elsewhere" {
		t.Fatalf("sync name = %q, want the one the instance names", got.Sync.Name)
	}
	if got.Sync.Kind != "" {
		t.Fatalf("sync = %+v, want nothing found under that name", got.Sync)
	}
}

func TestOverviewSaysWhenThereIsNoSync(t *testing.T) {
	got := overviewFor(
		t,
		[]runtime.Object{controllerDeployment("kustomize-controller", "v2.9.4", 1)},
		nil, nil,
	)

	if got.Sync.Kind != "" {
		t.Fatalf("sync = %+v, want none", got.Sync)
	}
	if !got.Ready {
		t.Fatal("a cluster with healthy controllers and no sync should still read as ready")
	}
	if got.Summary != "the controllers are ready; no cluster sync was found" {
		t.Fatalf("summary = %q", got.Summary)
	}
}

func TestOverviewAddsUpControllerUsageAgainstRequests(t *testing.T) {
	got := overviewFor(
		t,
		[]runtime.Object{
			controllerDeployment("source-controller", "v2.9.4", 1),
			controllerPod("source-controller-abc", "100m", "64Mi"),
			controllerPod("kustomize-controller-def", "100m", "64Mi"),
		},
		nil,
		map[string]api.ResourceUsage{
			"flux-system/source-controller-abc":    {CPUMilli: 20, MemoryMi: 100},
			"flux-system/kustomize-controller-def": {CPUMilli: 11, MemoryMi: 40},
		},
	)

	if !got.Usage.Known {
		t.Fatal("usage came back unknown even though metrics were given")
	}
	if got.Usage.CPUMilli != 31 || got.Usage.MemoryMi != 140 {
		t.Fatalf("usage = %+v", got.Usage)
	}
	if got.Usage.CPURequestMilli != 200 || got.Usage.MemRequestMi != 128 {
		t.Fatalf("requests = %+v", got.Usage)
	}
	if got.Usage.MemLimitMi != 2048 {
		t.Fatalf("limits = %+v", got.Usage)
	}
}

func TestOverviewLeavesUsageUnknownWithoutMetrics(t *testing.T) {
	got := overviewFor(t, []runtime.Object{
		controllerDeployment("source-controller", "v2.9.4", 1),
		controllerPod("source-controller-abc", "100m", "64Mi"),
	}, nil, nil)

	if got.Usage.Known {
		t.Fatal("usage was reported without metrics")
	}
}

func TestOverviewIsEmptyOnAClusterWithoutFlux(t *testing.T) {
	got := overviewFor(t, nil, nil, nil)

	if len(got.Controllers) != 0 {
		t.Fatalf("controllers = %+v, want none", got.Controllers)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want none for a cluster that simply has no flux", got.Error)
	}
}

func TestOverviewReportsARefusedControllerList(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("deployments is forbidden")
	})
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), overviewKinds())

	got := Overview(context.Background(), cs, listerFor(dyn), overviewDescs(), Cluster{})

	if got.Error == "" {
		t.Fatal("a refused list came back clean")
	}
}

type emptyLister struct{}

func (emptyLister) List(context.Context, api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	return nil, nil
}

func (emptyLister) Warm(context.Context, []api.ResourceDescriptor) {}

type failingLister struct{}

func (failingLister) List(context.Context, api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	return nil, errors.New("the cache could not be read")
}

func (failingLister) Warm(context.Context, []api.ResourceDescriptor) {}

// the small decisions the overview is assembled from

func TestWantedReplicasFallsBackToOne(t *testing.T) {
	if got := wantedReplicas(nil); got != 1 {
		t.Fatalf("wanted = %d, want 1 when the deployment names no replica count", got)
	}
	three := int32(3)
	if got := wantedReplicas(&three); got != 3 {
		t.Fatalf("wanted = %d, want 3", got)
	}
}

func TestSharedVersionOnlyAnswersWhenEveryControllerAgrees(t *testing.T) {
	cases := []struct {
		name        string
		controllers []api.FluxController
		want        string
	}{
		{name: "none at all", controllers: nil, want: ""},
		{
			name:        "one that never said",
			controllers: []api.FluxController{{Name: "source-controller"}},
			want:        "",
		},
		{
			name: "an unversioned one alongside a versioned one",
			controllers: []api.FluxController{
				{Name: "source-controller"},
				{Name: "kustomize-controller", Version: "v2.7.1"},
			},
			want: "v2.7.1",
		},
		{
			name: "two that disagree",
			controllers: []api.FluxController{
				{Name: "source-controller", Version: "v2.7.1"},
				{Name: "kustomize-controller", Version: "v2.6.0"},
			},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sharedVersion(tc.controllers); got != tc.want {
				t.Fatalf("version = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFluxInstanceIsNilWhenTheKindWasNeverDiscovered(t *testing.T) {
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("apps", "v1", "deployments"): {Group: "apps", Version: "v1", Resource: "deployments"},
	}

	if got := fluxInstance(context.Background(), emptyLister{}, descs); got != nil {
		t.Fatalf("instance = %+v, want nil without the instance kind", got)
	}
}

func TestSyncIsEmptyWithoutTheKustomizationKind(t *testing.T) {
	descs := map[string]api.ResourceDescriptor{}

	sync := syncOf(context.Background(), emptyLister{}, descs, "flux-system", "flux-system")

	if sync.Kind != "" {
		t.Fatalf("kind = %q, want empty without the kustomization kind", sync.Kind)
	}
	if sync.Namespace != "flux-system" || sync.Name != "flux-system" {
		t.Fatalf("sync = %+v, want it to still name what it looked for", sync)
	}
}

func TestSyncSourceNeedsBothAKindAndAName(t *testing.T) {
	entry := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"sourceRef": map[string]any{"kind": "GitRepository"}},
	}}

	kind, url, ref := syncSource(context.Background(), emptyLister{}, nil, entry, "flux-system")

	if kind != "" || url != "" || ref != "" {
		t.Fatalf("source = %q %q %q, want nothing without a source name", kind, url, ref)
	}
}

func TestSyncSourceKeepsTheKindWhenTheSourceCannotBeRead(t *testing.T) {
	entry := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"sourceRef": map[string]any{"kind": "GitRepository", "name": "flux-system"},
		},
	}}
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("source.toolkit.fluxcd.io", "v1", "gitrepositories"): {
			Group:    "source.toolkit.fluxcd.io",
			Version:  "v1",
			Resource: "gitrepositories",
		},
	}

	kind, url, ref := syncSource(context.Background(), failingLister{}, descs, entry, "flux-system")

	if kind != "GitRepository" {
		t.Fatalf("kind = %q, want the kind the sync named", kind)
	}
	if url != "" || ref != "" {
		t.Fatalf("url/ref = %q %q, want nothing when the source could not be read", url, ref)
	}
}

func TestRefOfHasNothingToSayWithoutARef(t *testing.T) {
	source := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{}}}

	if got := refOf(source); got != "" {
		t.Fatalf("ref = %q, want empty", got)
	}
}

func TestReadyConditionIgnoresWhatIsNotACondition(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"conditions": []any{
				"not a condition",
				map[string]any{"type": "Reconciling", "status": "True"},
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	}}

	if !readyCondition(item) {
		t.Fatal("ready = false, want the Ready condition to be found past the noise")
	}
}

func TestUsageStaysUnknownWhenTheControllerPodsCannotBeListed(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("pods are forbidden")
	})
	usage := map[string]api.ResourceUsage{"flux-system/source-controller-0": {CPUMilli: 5}}

	result := usageOf(context.Background(), cs, "flux-system", usage)

	if result.Known {
		t.Fatalf("usage = %+v, want it unknown when the pods could not be listed", result)
	}
}

func TestVerdictNamesAnUnreadySync(t *testing.T) {
	overview := api.FluxOverview{
		Controllers: []api.FluxController{{Name: "source-controller", Ready: true}},
		Sync:        api.FluxSync{Kind: "Kustomization", Ready: false},
	}

	ready, summary := verdict(overview)

	if ready {
		t.Fatal("ready = true, want false while the sync is not ready")
	}
	if summary != "the cluster sync is not ready" {
		t.Fatalf("summary = %q", summary)
	}
}
