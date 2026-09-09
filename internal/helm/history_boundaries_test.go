package helm

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestHistoryReportsAMissingMetadataClient(t *testing.T) {
	service := NewService(k8sfake.NewClientset(), nil, nil, nil, nil, api.ContextRef{})

	_, err := service.History(t.Context(), "demo", "podinfo", 0)

	if !errors.Is(err, errNoMetadata) {
		t.Fatalf("history error = %v, want the missing metadata client", err)
	}
}

func TestRevisionSelectorIncludesEveryRequestedRevision(t *testing.T) {
	selector := revisionSelector("podinfo", []int64{7, 4, 1})
	want := "owner=helm,name=podinfo,version in (7,4,1)"

	if selector != want {
		t.Fatalf("selector = %q, want %q", selector, want)
	}
}

func TestOneRefRejectsSameDriverDuplicateRevision(t *testing.T) {
	refs := []storedRef{
		{revision: 2, driver: DriverSecret, object: "sh.helm.release.v1.podinfo.v2-copy"},
		{revision: 2, driver: DriverSecret, object: "sh.helm.release.v1.podinfo.v2"},
	}

	_, err := oneRef(refs, "demo", "podinfo")

	if !errors.Is(err, errAmbiguousRevision) {
		t.Fatalf("revision error = %v, want the same-driver duplicate rejected", err)
	}
}

func TestDetailReportsARevisionThatDisappearsAfterMetadataListing(t *testing.T) {
	spec := sampleRelease()
	secret := detailSecret(spec)
	meta := metadatafake.NewSimpleMetadataClient(metaScheme(), metaOf(secret))
	service := serviceWithMeta(k8sfake.NewClientset(), meta, nil)

	_, err := service.Detail(t.Context(), "demo", "podinfo", spec.revision, resolver)

	if err == nil {
		t.Fatal("a revision missing from typed storage reported a readable detail")
	}
}

func TestDetailReportsAMetadataListFailure(t *testing.T) {
	meta := metadatafake.NewSimpleMetadataClient(metaScheme())
	meta.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("metadata unavailable")
	})
	service := serviceWithMeta(k8sfake.NewClientset(), meta, nil)

	_, err := service.Detail(t.Context(), "demo", "podinfo", 2, resolver)

	if err == nil || !strings.Contains(err.Error(), "metadata unavailable") {
		t.Fatalf("detail error = %v, want the metadata refusal", err)
	}
}

func TestHistoryReportsMetadataListFailures(t *testing.T) {
	for _, resource := range []string{"secrets", "configmaps"} {
		t.Run(resource, func(t *testing.T) {
			meta := metadatafake.NewSimpleMetadataClient(metaScheme())
			meta.PrependReactor("list", resource, func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New(resource + " list refused")
			})
			service := serviceWithMeta(k8sfake.NewClientset(), meta, nil)

			_, err := service.History(t.Context(), "demo", "podinfo", 0)

			if err == nil || !strings.Contains(err.Error(), resource+" list refused") {
				t.Fatalf("history error = %v, want the %s refusal", err, resource)
			}
		})
	}
}

func TestHistoryRejectsInvalidCoordinatesBeforeListing(t *testing.T) {
	service := NewService(k8sfake.NewClientset(), nil, nil, nil, nil, api.ContextRef{})
	for _, coordinates := range [][2]string{
		{"Not A Namespace", "podinfo"},
		{"demo", "--kubeconfig=/etc/shadow"},
	} {
		_, err := service.History(t.Context(), coordinates[0], coordinates[1], 0)

		if err == nil || errors.Is(err, errNoMetadata) {
			t.Fatalf("History(%q, %q) error = %v, want invalid coordinates", coordinates[0], coordinates[1], err)
		}
	}
}

func TestHistoryUsesTheLatestRevisionWhenNoCursorIsGiven(t *testing.T) {
	objects := make([]runtime.Object, 0, 3)
	for revision := int64(1); revision <= 3; revision++ {
		spec := sampleRelease()
		spec.revision = revision
		objects = append(objects, detailSecret(spec))
	}
	service := newService(k8sfake.NewClientset(objects...), nil, nil)

	page, err := service.History(t.Context(), "demo", "podinfo", 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}

	if len(page.Revisions) != 3 {
		t.Fatalf("history = %+v, want all three revisions", page.Revisions)
	}
	if page.Revisions[0].Revision != 3 || page.Revisions[2].Revision != 1 {
		t.Fatalf("history = %+v, want newest first", page.Revisions)
	}
	if page.Next != 0 {
		t.Fatalf("next = %d, want the end of history", page.Next)
	}
}

func TestHistoryTreatsAnOnlyMalformedRevisionAsMissing(t *testing.T) {
	bad := releaseSecret("sh.helm.release.v1.web.vbad", "not-a-number")
	service := newService(k8sfake.NewClientset(bad), nil, nil)

	_, err := service.History(t.Context(), "prod", "web", 0)

	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("history error = %v, want no usable release", err)
	}
}

func TestHistorySkipsAMalformedRevisionBesideAValidOne(t *testing.T) {
	valid := sampleRelease()
	valid.revision = 2
	bad := detailSecret(sampleRelease())
	bad.Name = "sh.helm.release.v1.podinfo.vbad"
	bad.Labels[versionLabel] = "not-a-number"
	service := newService(k8sfake.NewClientset(detailSecret(valid), bad), nil, nil)

	page, err := service.History(t.Context(), "demo", "podinfo", 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}

	if len(page.Revisions) != 1 || page.Revisions[0].Revision != 2 {
		t.Fatalf("history = %+v, want only revision 2", page.Revisions)
	}
}

func TestHistoryCanReturnAnEmptyPageBeforeTheOldestRevision(t *testing.T) {
	newest := sampleRelease()
	newest.revision = 3
	older := sampleRelease()
	older.revision = 2
	service := newService(k8sfake.NewClientset(detailSecret(newest), detailSecret(older)), nil, nil)

	page, err := service.History(t.Context(), "demo", "podinfo", 1)
	if err != nil {
		t.Fatalf("history: %v", err)
	}

	if len(page.Revisions) != 0 || page.Next != 0 {
		t.Fatalf("page = %+v, want the end of history", page)
	}
}

func TestHistoryRejectsAnOlderRevisionStoredTwice(t *testing.T) {
	newest := sampleRelease()
	newest.revision = 3
	older := sampleRelease()
	older.revision = 2
	secret := detailSecret(older)
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: secret.Namespace,
			Labels:    secret.Labels,
		},
		Data: map[string]string{releaseKey: string(secret.Data[releaseKey])},
	}
	service := newService(k8sfake.NewClientset(detailSecret(newest), secret, entry), nil, nil)

	_, err := service.History(t.Context(), "demo", "podinfo", 0)

	if !errors.Is(err, errAmbiguousRevision) {
		t.Fatalf("history error = %v, want the older duplicate rejected", err)
	}
}

func TestStoredRevisionMetadataReportsATruncatedDriverPage(t *testing.T) {
	meta := metadatafake.NewSimpleMetadataClient(metaScheme())
	meta.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		list := &metav1.List{}
		for revision := range 4 {
			list.Items = append(list.Items, runtime.RawExtension{Object: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.podinfo.v" + strconv.Itoa(revision+1),
					Namespace: "demo",
					Labels: map[string]string{
						"owner": "helm", nameLabel: "podinfo", versionLabel: strconv.Itoa(revision + 1),
					},
				},
			}})
		}
		return true, list, nil
	})
	service := serviceWithMeta(k8sfake.NewClientset(), meta, nil)

	_, err := service.storedRefs(t.Context(), "demo", releaseSelector("podinfo"), 2)

	if err == nil || !strings.Contains(err.Error(), "more than 2") {
		t.Fatalf("stored refs error = %v, want the truncation reported", err)
	}
}

func TestTypedRevisionScanRequiresAPositiveLimit(t *testing.T) {
	_, err := revisionsMatching(t.Context(), k8sfake.NewClientset(), "prod", ownerLabel, 0)

	if err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("revision scan error = %v, want the invalid limit", err)
	}
}

func TestTypedRevisionScanReportsStorageListFailures(t *testing.T) {
	for _, resource := range []string{"secrets", "configmaps"} {
		t.Run(resource, func(t *testing.T) {
			client := k8sfake.NewClientset()
			client.PrependReactor("list", resource, func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New(resource + " list refused")
			})

			_, err := revisionsIn(t.Context(), client, "prod", "web")

			if err == nil || !strings.Contains(err.Error(), resource+" list refused") {
				t.Fatalf("revision scan error = %v, want the %s refusal", err, resource)
			}
		})
	}
}

func TestTypedRevisionScanSkipsObjectsThatAreNotReleasePayloads(t *testing.T) {
	ordinary := releaseSecret("ordinary", "1")
	ordinary.Type = corev1.SecretTypeOpaque
	validSecret := releaseSecret("sh.helm.release.v1.web.v2", "2")
	missingBody := releaseConfigMap("missing-body", "3")
	missingBody.Data = map[string]string{}
	validMap := releaseConfigMap("sh.helm.release.v1.web.v4", "4")
	client := k8sfake.NewClientset(ordinary, validSecret, missingBody, validMap)

	found, err := revisionsIn(t.Context(), client, "prod", "web")
	if err != nil {
		t.Fatalf("revisions: %v", err)
	}

	if len(found) != 2 {
		t.Fatalf("revisions = %+v, want only the two release payloads", found)
	}
	if found[0].revision != 2 || found[1].revision != 4 {
		t.Fatalf("revisions = %+v, want 2 and 4", found)
	}
}

func TestTypedRevisionScanCapsCombinedStorageDrivers(t *testing.T) {
	client := k8sfake.NewClientset(
		releaseSecret("sh.helm.release.v1.web.v1", "1"),
		releaseConfigMap("sh.helm.release.v1.web.v2", "2"),
	)

	_, err := revisionsMatching(t.Context(), client, "prod", ownerLabel, 1)

	if err == nil || !strings.Contains(err.Error(), "more than 1") {
		t.Fatalf("revision scan error = %v, want the combined object cap", err)
	}
}

func TestTypedRevisionScansCapEveryObjectTheyInspect(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*k8sfake.Clientset) error
		objs []runtime.Object
	}{
		{
			name: "secrets",
			run: func(client *k8sfake.Clientset) error {
				_, err := revisionSecrets(t.Context(), client, "prod", ownerLabel, 1)
				return err
			},
			objs: []runtime.Object{
				releaseSecret("sh.helm.release.v1.web.v1", "1"),
				releaseSecret("sh.helm.release.v1.web.v2", "2"),
			},
		},
		{
			name: "configmaps",
			run: func(client *k8sfake.Clientset) error {
				_, err := revisionConfigMaps(t.Context(), client, "prod", ownerLabel, 1)
				return err
			},
			objs: []runtime.Object{
				releaseConfigMap("sh.helm.release.v1.web.v1", "1"),
				releaseConfigMap("sh.helm.release.v1.web.v2", "2"),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.run(k8sfake.NewClientset(test.objs...))

			if err == nil || !strings.Contains(err.Error(), "more than 1") {
				t.Fatalf("scan error = %v, want the object cap", err)
			}
		})
	}
}

func TestActionsUseTypedStorageWhenMetadataIsUnavailable(t *testing.T) {
	runner := &stubRunner{}
	old := sampleRelease()
	old.revision = 1
	newest := sampleRelease()
	newest.revision = 2
	secret := detailSecret(old)
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1.podinfo.v2",
			Namespace: "demo",
			Labels: map[string]string{
				"owner": "helm", "name": "podinfo", versionLabel: "2",
			},
		},
		Data: map[string]string{releaseKey: detailPayload(newest)},
	}
	service := NewService(
		k8sfake.NewClientset(secret, entry),
		nil,
		runner,
		nil,
		nil,
		api.ContextRef{Name: "kind-spinoza"},
	)

	_, err := service.Uninstall(context.Background(), "demo", "podinfo")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if len(runner.envs) != 1 || len(runner.envs[0]) != 1 || runner.envs[0][0] != driverEnv+"="+DriverConfigMap {
		t.Fatalf("env = %v, want the newest typed-storage driver", runner.envs)
	}
}

func TestActionsReportTypedStorageFailuresWithoutRunningHelm(t *testing.T) {
	for _, test := range []struct {
		name    string
		client  func() *k8sfake.Clientset
		missing bool
	}{
		{
			name: "storage list failure",
			client: func() *k8sfake.Clientset {
				client := k8sfake.NewClientset()
				client.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("typed storage unavailable")
				})
				return client
			},
		},
		{
			name: "missing release",
			client: func() *k8sfake.Clientset {
				return k8sfake.NewClientset()
			},
			missing: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &stubRunner{}
			service := NewService(
				test.client(),
				nil,
				runner,
				nil,
				nil,
				api.ContextRef{Name: "kind-spinoza"},
			)

			_, err := service.Uninstall(t.Context(), "demo", "podinfo")

			if err == nil {
				t.Fatal("uninstall succeeded without a usable storage driver")
			}
			if test.missing && !errors.Is(err, ErrNoRelease) {
				t.Fatalf("uninstall error = %v, want a missing release", err)
			}
			if !test.missing && !strings.Contains(err.Error(), "typed storage unavailable") {
				t.Fatalf("uninstall error = %v, want the storage refusal", err)
			}
			if len(runner.args) != 0 {
				t.Fatalf("helm ran %d times without a usable storage driver", len(runner.args))
			}
		})
	}
}

func TestActionsReportMetadataFailureWithoutRunningHelm(t *testing.T) {
	meta := metadatafake.NewSimpleMetadataClient(metaScheme())
	meta.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("metadata unavailable")
	})
	runner := &stubRunner{}
	service := NewService(
		k8sfake.NewClientset(),
		meta,
		runner,
		nil,
		nil,
		api.ContextRef{Name: "kind-spinoza"},
	)

	_, err := service.Uninstall(t.Context(), "demo", "podinfo")

	if err == nil || !strings.Contains(err.Error(), "metadata unavailable") {
		t.Fatalf("uninstall error = %v, want the metadata refusal", err)
	}
	if len(runner.args) != 0 {
		t.Fatalf("helm ran %d times after metadata failed", len(runner.args))
	}
}

func TestActionsRejectAmbiguousTypedStorageWithoutMetadata(t *testing.T) {
	runner := &stubRunner{}
	spec := sampleRelease()
	secret := detailSecret(spec)
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: secret.Namespace,
			Labels:    secret.Labels,
		},
		Data: map[string]string{releaseKey: string(secret.Data[releaseKey])},
	}
	service := NewService(
		k8sfake.NewClientset(secret, entry),
		nil,
		runner,
		nil,
		nil,
		api.ContextRef{Name: "kind-spinoza"},
	)

	_, err := service.Uninstall(context.Background(), "demo", "podinfo")

	if !errors.Is(err, errAmbiguousRevision) {
		t.Fatalf("uninstall error = %v, want ambiguous storage", err)
	}
	if len(runner.args) != 0 {
		t.Fatalf("helm ran %d times for ambiguous storage", len(runner.args))
	}
}
