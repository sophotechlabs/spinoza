package helm

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"
)

func pageOf(items int) func(k8stesting.Action) (bool, runtime.Object, error) {
	return func(k8stesting.Action) (bool, runtime.Object, error) {
		list := &metav1.List{ListMeta: metav1.ListMeta{Continue: "there-is-more"}}
		for i := range items {
			list.Items = append(list.Items, runtime.RawExtension{Object: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("sh.helm.release.v1.web.v%d", i),
					Namespace: "prod",
					Labels:    map[string]string{"owner": "helm"},
				},
			}})
		}
		return true, list, nil
	}
}

func TestReadingStopsAtTheObjectCapAndSaysSo(t *testing.T) {
	client := metadatafake.NewSimpleMetadataClient(metaScheme())
	client.PrependReactor("list", "secrets", pageOf(maxObjects))

	page, err := listRefs(t.Context(), client, DriverSecret, secretsGVR, "prod")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.items) != maxObjects {
		t.Fatalf("read %d objects, want it to stop at the cap of %d", len(page.items), maxObjects)
	}
	if !page.truncated {
		t.Fatal("a list that stopped early did not say it was truncated")
	}
}

func TestAListThatFitsIsNotCalledTruncated(t *testing.T) {
	client := metadatafake.NewSimpleMetadataClient(metaScheme(),
		&metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.web.v1",
				Namespace: "prod",
				Labels:    map[string]string{"owner": "helm"},
			},
		})

	page, err := listRefs(t.Context(), client, DriverSecret, secretsGVR, "prod")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.truncated {
		t.Fatal("a list that read everything reported itself truncated")
	}
}

func TestTruncationTravelsUpToTheWholeRead(t *testing.T) {
	client := metadatafake.NewSimpleMetadataClient(metaScheme(),
		&metav1.PartialObjectMetadata{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
			ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		})
	client.PrependReactor("list", "secrets", pageOf(maxObjects))

	page, err := allRefs(t.Context(), client)
	if err != nil {
		t.Fatalf("refs: %v", err)
	}
	if !page.truncated {
		t.Fatal("a read that stopped at the cap was reported as complete")
	}
}

func TestANamespaceFallbackThatHitsTheCapStillSaysItWasTruncated(t *testing.T) {
	client := metadatafake.NewSimpleMetadataClient(metaScheme(), namespaceMeta("prod"))
	client.PrependReactor("list", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == "" {
			return true, nil, forbiddenSecrets("no cluster-wide secrets")
		}
		return pageOf(maxObjects)(action)
	})

	page, err := allRefs(t.Context(), client)
	if err != nil {
		t.Fatalf("refs: %v", err)
	}
	if len(page.items) != maxObjects {
		t.Fatalf("read %d objects, want the cap of %d", len(page.items), maxObjects)
	}
	if !page.truncated {
		t.Fatal("a namespace fallback that stopped at the cap was reported as complete")
	}
}
