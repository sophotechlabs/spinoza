package inspect

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func secret(data map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]any{"name": "creds", "namespace": "prod"},
		"data":       data,
	}}
}

// what the browser is given for a secret

func TestEachKeyArrivesDecoded(t *testing.T) {
	//nolint:gosec // the fixture is base64 of "admin" and "hunter2", not a credential
	got := secretDataOf(secret(map[string]any{
		"username": "YWRtaW4=",
		"password": "aHVudGVyMg==",
	}))

	want := []api.SecretEntry{
		{Key: "password", Value: "hunter2", Bytes: 7},
		{Key: "username", Value: "admin", Bytes: 5},
	}
	if len(got) != len(want) {
		t.Fatalf("entries = %d, want %d", len(got), len(want))
	}
	for i, entry := range got {
		if entry != want[i] {
			t.Fatalf("entry %d = %+v, want %+v", i, entry, want[i])
		}
	}
}

func TestKeysComeBackInOrder(t *testing.T) {
	got := secretDataOf(secret(map[string]any{"c": "Yw==", "a": "YQ==", "b": "Yg=="}))

	if got[0].Key != "a" || got[1].Key != "b" || got[2].Key != "c" {
		t.Fatalf("keys = %q %q %q, want them sorted", got[0].Key, got[1].Key, got[2].Key)
	}
}

func TestBytesThatAreNotTextAreFlagged(t *testing.T) {
	got := secretDataOf(secret(map[string]any{"keystore": "/v8A"}))

	if !got[0].Binary {
		t.Fatalf("entry = %+v, want it marked binary", got[0])
	}
	if got[0].Value != "/v8A" {
		t.Fatalf("value = %q, want the base64 left as it came", got[0].Value)
	}
	if got[0].Bytes != 3 {
		t.Fatalf("bytes = %d, want the decoded length", got[0].Bytes)
	}
}

func TestSomethingThatIsNotBase64IsLeftAlone(t *testing.T) {
	got := secretDataOf(secret(map[string]any{"broken": "not base64!"}))

	if !got[0].Binary || got[0].Value != "not base64!" {
		t.Fatalf("entry = %+v, want the raw value marked binary", got[0])
	}
}

func TestAValueThatIsNotAStringIsSkipped(t *testing.T) {
	got := secretDataOf(secret(map[string]any{"odd": int64(3), "fine": "YQ=="}))

	if len(got) != 1 || got[0].Key != "fine" {
		t.Fatalf("entries = %+v, want only the string one", got)
	}
}

// what other kinds get

func TestOnlySecretsCarryData(t *testing.T) {
	configmap := secret(map[string]any{"a": "YQ=="})
	configmap.SetKind("ConfigMap")

	if secretDataOf(configmap) != nil {
		t.Fatal("a configmap was given decoded secret data")
	}
}

func TestASecretWithNoDataCarriesNone(t *testing.T) {
	bare := secret(nil)
	unstructured.RemoveNestedField(bare.Object, "data")

	if secretDataOf(bare) != nil {
		t.Fatal("a secret with no data still produced entries")
	}
}
