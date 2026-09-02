package resources

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/helm"
)

func helmStorageSecret(namespace, name string) *corev1.Secret {
	payload := `{"name":"` + name + `","namespace":"` + namespace + `","version":1,` +
		`"info":{"status":"deployed"},"chart":{"metadata":{"name":"podinfo","version":"6.9.2"}}}`
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1." + name + ".v1",
			Namespace: namespace,
			Labels:    map[string]string{"owner": "helm", "name": name, "version": "1", "status": "deployed"},
		},
		Type: "helm.sh/release.v1",
		Data: map[string][]byte{"release": []byte(payload)},
	}
}

func helmReleaseCR(namespace, name string, spec map[string]any) *unstructured.Unstructured {
	object := map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}
	if spec != nil {
		object["spec"] = spec
	}
	return &unstructured.Unstructured{Object: object}
}

func fluxManager(t *testing.T, secrets []*corev1.Secret, crs ...*unstructured.Unstructured) *Manager {
	t.Helper()
	gvr := schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
	kinds := map[schema.GroupVersionResource]string{gvr: "HelmReleaseList"}
	objs := make([]runtime.Object, 0, len(crs))
	for _, cr := range crs {
		objs = append(objs, cr)
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
	seeds := make([]runtime.Object, 0, len(secrets))
	for _, secret := range secrets {
		seeds = append(seeds, secret)
	}
	cs := k8sfake.NewClientset(seeds...)
	releases := helm.NewService(cs, helmMeta(t, cs), nil, nil, nil, api.ContextRef{Name: "kind-spinoza"})
	descs := map[string]api.ResourceDescriptor{
		discovery.Key("helm.toolkit.fluxcd.io", "v2", "helmreleases"): {
			Group:      "helm.toolkit.fluxcd.io",
			Version:    "v2",
			Resource:   "helmreleases",
			Kind:       "HelmRelease",
			Namespaced: true,
		},
	}
	return NewManager(t.Context(), Deps{Dynamic: dyn, Clientset: cs, Helm: releases, Descriptors: descs})
}

func TestHelmReleasesCarryTheirFluxOwner(t *testing.T) {
	mgr := fluxManager(
		t,
		[]*corev1.Secret{helmStorageSecret("demo", "podinfo")},
		helmReleaseCR("demo", "podinfo", nil),
	)

	got, err := mgr.HelmReleases(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	ref := got.Releases[0].FluxRef
	if ref == nil {
		t.Fatal("a flux-owned release carried no owner ref")
	}
	if ref.Group != "helm.toolkit.fluxcd.io" || ref.Version != "v2" || ref.Resource != "helmreleases" {
		t.Fatalf("ref = %+v, want the resolved helmreleases gvr", ref)
	}
	if ref.Namespace != "demo" || ref.Name != "podinfo" {
		t.Fatalf("ref = %+v, want the owning object's identity", ref)
	}
}

func TestHelmReleasesMatchAComposedFluxReleaseName(t *testing.T) {
	mgr := fluxManager(
		t,
		[]*corev1.Secret{helmStorageSecret("flux-system", "demo-podinfo")},
		helmReleaseCR("flux-system", "podinfo", map[string]any{"targetNamespace": "demo"}),
	)

	got, err := mgr.HelmReleases(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if got.Releases[0].FluxRef == nil {
		t.Fatal("a release stored under the composed [targetNamespace-]name was not matched")
	}
}

func TestHelmReleasesLeaveAHandInstalledReleaseAlone(t *testing.T) {
	mgr := fluxManager(
		t,
		[]*corev1.Secret{helmStorageSecret("demo", "podinfo")},
		helmReleaseCR("flux-system", "something-else", nil),
	)

	got, err := mgr.HelmReleases(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if got.Releases[0].FluxRef != nil {
		t.Fatalf("fluxRef = %+v, want none for a hand-installed release", got.Releases[0].FluxRef)
	}
}

func TestHelmReleaseDetailCarriesTheFluxOwner(t *testing.T) {
	mgr := fluxManager(
		t,
		[]*corev1.Secret{helmStorageSecret("demo", "podinfo")},
		helmReleaseCR("demo", "podinfo", nil),
	)

	got, err := mgr.HelmRelease(context.Background(), "demo", "podinfo", 0)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if got.Release.FluxRef == nil {
		t.Fatal("the detail view lost the owner ref")
	}
}

func TestHelmUpgradeRefusesAFluxOwnedRelease(t *testing.T) {
	mgr := fluxManager(
		t,
		[]*corev1.Secret{helmStorageSecret("demo", "podinfo")},
		helmReleaseCR("demo", "podinfo", nil),
	)

	_, err := mgr.HelmUpgrade(context.Background(), helm.UpgradeRequest{
		Namespace: "demo",
		Name:      "podinfo",
		Chart:     "podinfo",
		Version:   "6.10.0",
		RepoURL:   "https://charts.example.com",
	})

	if !errors.Is(err, helm.ErrFluxManaged) {
		t.Fatalf("err = %v, want ErrFluxManaged", err)
	}
	if !strings.Contains(err.Error(), "demo/podinfo") {
		t.Fatalf("err = %v, want the owning object named", err)
	}
}

func TestHelmUpgradeWithoutAFluxOwnerReachesTheService(t *testing.T) {
	mgr := fluxManager(t, []*corev1.Secret{helmStorageSecret("demo", "podinfo")})

	_, err := mgr.HelmUpgrade(context.Background(), helm.UpgradeRequest{
		Namespace: "demo",
		Name:      "podinfo",
		Chart:     "podinfo",
		Version:   "6.10.0",
		RepoURL:   "https://charts.example.com",
	})

	if err == nil {
		t.Fatal("a service with no runner reported success")
	}
	if errors.Is(err, helm.ErrFluxManaged) {
		t.Fatal("an unowned release was refused as flux-managed")
	}
}

func TestHelmUpgradeAndVersionsSayWhenHelmIsNotWiredUp(t *testing.T) {
	mgr := viewManager(t, nil)

	_, upgradeErr := mgr.HelmUpgrade(context.Background(), helm.UpgradeRequest{})
	_, versionsErr := mgr.HelmVersions(context.Background(), "podinfo")

	if !errors.Is(upgradeErr, api.ErrInternal) {
		t.Fatalf("upgrade err = %v, want an internal failure", upgradeErr)
	}
	if !errors.Is(versionsErr, api.ErrInternal) {
		t.Fatalf("versions err = %v, want an internal failure", versionsErr)
	}
}

func TestHelmVersionsReachTheService(t *testing.T) {
	mgr := fluxManager(t, nil)

	got, err := mgr.HelmVersions(context.Background(), "podinfo")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}

	if got.Chart != "podinfo" {
		t.Fatalf("chart = %q", got.Chart)
	}
	if got.Error == "" {
		t.Fatal("a service with no repositories reported nothing to say")
	}
}

func helmMeta(t *testing.T, cs kubernetes.Interface) metadata.Interface {
	t.Helper()
	scheme := runtime.NewScheme()
	err := metav1.AddMetaToScheme(scheme)
	if err != nil {
		t.Fatalf("scheme: %v", err)
	}
	objs := []runtime.Object{}
	secrets, listErr := cs.CoreV1().Secrets("").List(context.Background(), metav1.ListOptions{})
	if listErr == nil {
		for i := range secrets.Items {
			objs = append(objs, &metav1.PartialObjectMetadata{
				TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
				ObjectMeta: secrets.Items[i].ObjectMeta,
			})
		}
	}
	return metadatafake.NewSimpleMetadataClient(scheme, objs...)
}
