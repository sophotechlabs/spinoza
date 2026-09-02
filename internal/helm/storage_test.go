package helm

import (
	"errors"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func releaseSecret(name, revision string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "prod",
			Name:      name,
			Labels:    map[string]string{"owner": "helm", "name": "web", versionLabel: revision},
		},
		Type: storageType,
		Data: map[string][]byte{releaseKey: []byte("body")},
	}
}

func releaseConfigMap(name, revision string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "prod",
			Name:      name,
			Labels:    map[string]string{"owner": "helm", "name": "web", versionLabel: revision},
		},
		Data: map[string]string{releaseKey: "body"},
	}
}

func pagesOf(cs *k8sfake.Clientset, resource string, pages [][]runtime.Object) {
	at := 0
	cs.PrependReactor("list", resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
		page := pages[at]
		more := ""
		if at < len(pages)-1 {
			at++
			more = strconv.Itoa(at)
		}
		switch resource {
		case "secrets":
			list := &corev1.SecretList{ListMeta: metav1.ListMeta{Continue: more}}
			for _, item := range page {
				secret, ok := item.(*corev1.Secret)
				if ok {
					list.Items = append(list.Items, *secret)
				}
			}
			return true, list, nil
		default:
			list := &corev1.ConfigMapList{ListMeta: metav1.ListMeta{Continue: more}}
			for _, item := range page {
				entry, ok := item.(*corev1.ConfigMap)
				if ok {
					list.Items = append(list.Items, *entry)
				}
			}
			return true, list, nil
		}
	})
}

func TestEveryPageOfReleaseSecretsIsRead(t *testing.T) {
	cs := k8sfake.NewClientset()
	pagesOf(cs, "secrets", [][]runtime.Object{
		{releaseSecret("sh.helm.release.v1.web.v1", "1")},
		{releaseSecret("sh.helm.release.v1.web.v2", "2")},
		{releaseSecret("sh.helm.release.v1.web.v3", "3")},
	})

	found, err := revisionsIn(t.Context(), cs, "prod", "web")
	if err != nil {
		t.Fatalf("revisions: %v", err)
	}
	if len(found) != 3 {
		t.Fatalf("found %d revisions, want every page read", len(found))
	}
}

func TestEveryPageOfReleaseConfigMapsIsRead(t *testing.T) {
	cs := k8sfake.NewClientset()
	pagesOf(cs, "configmaps", [][]runtime.Object{
		{releaseConfigMap("sh.helm.release.v1.web.v1", "1")},
		{releaseConfigMap("sh.helm.release.v1.web.v2", "2")},
	})

	found, err := revisionsIn(t.Context(), cs, "prod", "web")
	if err != nil {
		t.Fatalf("revisions: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("found %d revisions, want both pages", len(found))
	}
}

func TestReleaseStorageRejectsARepeatedContinueToken(t *testing.T) {
	tests := []struct {
		name string
		read func(*k8sfake.Clientset) error
		page runtime.Object
	}{
		{
			name: "secrets",
			page: &corev1.SecretList{ListMeta: metav1.ListMeta{Continue: "same"}},
			read: func(cs *k8sfake.Clientset) error {
				_, err := revisionSecrets(t.Context(), cs, "prod", "owner=helm")
				return err
			},
		},
		{
			name: "configmaps",
			page: &corev1.ConfigMapList{ListMeta: metav1.ListMeta{Continue: "same"}},
			read: func(cs *k8sfake.Clientset) error {
				_, err := revisionConfigMaps(t.Context(), cs, "prod", "owner=helm")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cs := k8sfake.NewClientset()
			cs.PrependReactor("list", test.name, func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, test.page, nil
			})

			err := test.read(cs)

			if !errors.Is(err, errRepeatedContinue) {
				t.Fatalf("storage list error = %v, want the repeated token", err)
			}
		})
	}
}

func TestAConfigMapListThatFailsIsReported(t *testing.T) {
	cs := k8sfake.NewClientset(releaseSecret("sh.helm.release.v1.web.v1", "1"))
	cs.PrependReactor("list", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("configmaps is forbidden")
	})

	_, err := revisionsIn(t.Context(), cs, "prod", "web")

	if err == nil {
		t.Fatal("a refused configmap list was reported as a complete history")
	}
}

func TestARevisionWithoutANumberFallsBackToZero(t *testing.T) {
	cs := k8sfake.NewClientset(releaseSecret("sh.helm.release.v1.web.v1", "not a number"))

	found, err := revisionsIn(t.Context(), cs, "prod", "web")
	if err != nil {
		t.Fatalf("revisions: %v", err)
	}
	if len(found) != 1 || found[0].revision != 0 {
		t.Fatalf("found = %+v, want the revision kept at zero", found)
	}
}
