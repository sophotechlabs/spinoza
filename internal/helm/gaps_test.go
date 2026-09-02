package helm

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func namespaceMeta(name string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

func TestNotesAreJoinedOnlyWhenThereAreTwo(t *testing.T) {
	if got := joinNotes("", "config maps refused"); got != "config maps refused" {
		t.Fatalf("notes = %q, want the right one alone", got)
	}
	if got := joinNotes("secrets refused", ""); got != "secrets refused" {
		t.Fatalf("notes = %q, want the left one alone", got)
	}
	if got := joinNotes("a", "b"); got != "a; b" {
		t.Fatalf("notes = %q, want both", got)
	}
}

func TestOnlyTheNewestRevisionOfEachReleaseSurvives(t *testing.T) {
	refs := []storedRef{
		{namespace: "apps", name: "podinfo", object: "sh.helm.release.v1.podinfo.v2", revision: 2},
		{namespace: "apps", name: "podinfo", object: "sh.helm.release.v1.podinfo.v1", revision: 1},
		{namespace: "apps", name: "", object: "not-a-release"},
	}

	kept := newestPerRelease(refs)

	if len(kept) != 1 {
		t.Fatalf("kept = %d, want just the newest revision", len(kept))
	}
	if kept[0].revision != 2 {
		t.Fatalf("revision = %d, want 2", kept[0].revision)
	}
}

func TestTheReleaseCacheForgetsObjectsThatAreNoLongerListed(t *testing.T) {
	cache := newReleaseCache()
	kept := storedRef{driver: DriverSecret, namespace: "apps", object: "kept", version: "2"}
	removed := storedRef{driver: DriverSecret, namespace: "apps", object: "removed", version: "1"}
	cache.put(kept, api.HelmRelease{Name: "kept"})
	cache.put(removed, api.HelmRelease{Name: "removed"})

	cache.keep([]storedRef{kept})

	if _, ok := cache.get(removed); ok {
		t.Fatal("an object missing from the latest list remained cached")
	}
	if release, ok := cache.get(kept); !ok || release.Name != "kept" {
		t.Fatalf("the still-listed release was lost: %+v, %t", release, ok)
	}
}

func TestAListFailureThatIsNotForbiddenStopsTheWalk(t *testing.T) {
	cs := k8sfake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
	)
	meta := mirrorMeta(cs)
	meta.PrependReactor("list", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == "" {
			return true, nil, forbiddenSecrets("no cluster-wide secrets")
		}
		return true, nil, errors.New("the apiserver fell over")
	})

	_, err := serviceWithMeta(cs, meta, nil).List(context.Background())

	if err == nil {
		t.Fatal("List returned nil error when a namespace failed outright")
	}
	if !strings.Contains(err.Error(), "fell over") {
		t.Fatalf("error = %q, want the namespace failure", err.Error())
	}
}

func TestANamespaceListFailureOtherThanDenialIsReported(t *testing.T) {
	cs := k8sfake.NewClientset()
	meta := mirrorMeta(cs)
	meta.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, forbiddenSecrets("no cluster-wide secrets")
	})
	meta.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("namespace discovery fell over")
	})

	_, err := serviceWithMeta(cs, meta, nil).List(context.Background())

	if err == nil {
		t.Fatal("List returned nil error when namespace discovery failed")
	}
	if !strings.Contains(err.Error(), "namespace discovery fell over") {
		t.Fatalf("error = %q, want the namespace-list failure", err.Error())
	}
}

func TestTheNamespaceWalkFollowsItsContinueToken(t *testing.T) {
	cs := k8sfake.NewClientset()
	meta := mirrorMeta(cs)
	meta.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, forbiddenSecrets("no cluster-wide secrets")
	})
	pages := 0
	meta.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		pages++
		list := &metav1.List{}
		if pages == 1 {
			list.ListMeta = metav1.ListMeta{Continue: "next"}
			list.Items = []runtime.RawExtension{{Object: namespaceMeta("apps")}}
			return true, list, nil
		}
		list.Items = []runtime.RawExtension{{Object: namespaceMeta("flux-system")}}
		return true, list, nil
	})

	got, err := serviceWithMeta(cs, meta, nil).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if pages != 2 {
		t.Fatalf("namespace pages = %d, want the walk to follow the token", pages)
	}
	if !strings.Contains(got.Error, "2 namespaces") {
		t.Fatalf("note = %q, want both namespaces counted", got.Error)
	}
}

func TestTheNamespaceWalkRejectsARepeatedContinueToken(t *testing.T) {
	meta := mirrorMeta(k8sfake.NewClientset())
	meta.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &metav1.List{ListMeta: metav1.ListMeta{Continue: "same"}}, nil
	})

	_, err := namespaceRefs(t.Context(), meta)

	if !errors.Is(err, errRepeatedContinue) {
		t.Fatalf("namespace list error = %v, want the repeated token", err)
	}
}

func TestSecretsThatAreNotHelmStorageAreSkipped(t *testing.T) {
	other := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "unrelated", Labels: map[string]string{"owner": "helm"}},
		Type:       corev1.SecretTypeOpaque,
	}
	cs := k8sfake.NewClientset(other)

	found, err := revisionSecrets(context.Background(), cs, "apps", "owner=helm", maxObjects)
	if err != nil {
		t.Fatalf("revisionSecrets: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %d, want the non-helm secret skipped", len(found))
	}
}

func TestARefusedSecretListIsReported(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("secrets are forbidden")
	})

	_, err := revisionSecrets(context.Background(), cs, "apps", "owner=helm", maxObjects)

	if err == nil {
		t.Fatal("revisionSecrets returned nil error")
	}
}

func TestARefusedConfigMapListIsReported(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("list", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("config maps are forbidden")
	})

	_, err := revisionConfigMaps(context.Background(), cs, "apps", "owner=helm", maxObjects)

	if err == nil {
		t.Fatal("revisionConfigMaps returned nil error when config maps were refused")
	}
}

func TestAConfigMapWithoutAReleaseIsSkipped(t *testing.T) {
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "settings", Labels: map[string]string{"owner": "helm"}},
		Data:       map[string]string{"other": "value"},
	}
	cs := k8sfake.NewClientset(entry)

	found, err := revisionConfigMaps(context.Background(), cs, "apps", "owner=helm", maxObjects)
	if err != nil {
		t.Fatalf("revisionConfigMaps: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %d, want a config map with no release skipped", len(found))
	}
}

func TestAConfigMapBodyNeedsTheReleaseKey(t *testing.T) {
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "settings"},
		Data:       map[string]string{"other": "value"},
	}
	cs := k8sfake.NewClientset(entry)
	ref := storedRef{driver: DriverConfigMap, namespace: "apps", object: "settings"}

	_, err := configMapBody(context.Background(), cs, ref)

	if !errors.Is(err, errNotRelease) {
		t.Fatalf("error = %v, want it to say that is not a release", err)
	}
}

func TestAMissingConfigMapBodyIsReported(t *testing.T) {
	cs := k8sfake.NewClientset()
	ref := storedRef{driver: DriverConfigMap, namespace: "apps", object: "gone"}

	_, err := configMapBody(context.Background(), cs, ref)

	if err == nil {
		t.Fatal("configMapBody returned nil error for a config map that is not there")
	}
}

func TestAnUpgradeReportsWhatHelmSaid(t *testing.T) {
	cs := k8sfake.NewClientset(helmSecret(release{
		name: "podinfo", namespace: "apps", revision: 1, status: "deployed",
		chart: "podinfo", version: "6.9.1", appVersion: "6.9.1",
	}))
	runner := &stubRunner{err: errors.New("upgrade failed: timed out")}
	service := NewService(cs, mirrorMeta(cs), runner, nil, actionRepositories(), api.ContextRef{Name: "kind-spinoza"})
	req := UpgradeRequest{Namespace: "apps", Name: "podinfo", Chart: "podinfo", RepoURL: "https://example.com", Version: "6.9.2"}

	_, err := service.Upgrade(context.Background(), req)

	if err == nil {
		t.Fatal("Upgrade returned nil error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %q, want what helm said", err.Error())
	}
}

func TestAnUninstallReportsWhatHelmSaid(t *testing.T) {
	cs := k8sfake.NewClientset(helmSecret(release{
		name: "podinfo", namespace: "apps", revision: 1, status: "deployed",
		chart: "podinfo", version: "6.9.1", appVersion: "6.9.1",
	}))
	runner := &stubRunner{err: errors.New("uninstall failed: release not found")}
	service := NewService(cs, mirrorMeta(cs), runner, nil, nil, api.ContextRef{Name: "kind-spinoza"})

	_, err := service.Uninstall(context.Background(), "apps", "podinfo")

	if err == nil {
		t.Fatal("Uninstall returned nil error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want what helm said", err.Error())
	}
}

func TestValuesThatCannotBeWrittenStopTheUpgrade(t *testing.T) {
	t.Setenv("TMPDIR", "/spinoza-nowhere")
	cs := k8sfake.NewClientset(helmSecret(release{
		name: "podinfo", namespace: "apps", revision: 1, status: "deployed",
		chart: "podinfo", version: "6.9.1", appVersion: "6.9.1",
	}))
	service := NewService(
		cs,
		mirrorMeta(cs),
		&stubRunner{},
		nil,
		actionRepositories(),
		api.ContextRef{Name: "kind-spinoza"},
	)
	req := UpgradeRequest{Namespace: "apps", Name: "podinfo", Chart: "podinfo", RepoURL: "https://example.com", Version: "6.9.2", Values: "replicaCount: 2"}

	_, err := service.Upgrade(context.Background(), req)

	if err == nil {
		t.Fatal("Upgrade returned nil error when the values file could not be written")
	}
}
