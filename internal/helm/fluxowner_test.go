package helm

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestStorageKeyUsesTheExplicitReleaseName(t *testing.T) {
	cr := FluxRelease{CRNamespace: "flux-system", CRName: "podinfo-app", ReleaseName: "podinfo"}

	if cr.StorageKey() != "flux-system/podinfo" {
		t.Fatalf("key = %q, want the explicit release name", cr.StorageKey())
	}
}

func TestStorageKeyComposesTheTargetNamespaceIntoTheName(t *testing.T) {
	cr := FluxRelease{CRNamespace: "flux-system", CRName: "podinfo", TargetNamespace: "apps"}

	if cr.StorageKey() != "flux-system/apps-podinfo" {
		t.Fatalf("key = %q, want the [targetNamespace-]name composition", cr.StorageKey())
	}
}

func TestStorageKeyDefaultsToTheCRsOwnName(t *testing.T) {
	cr := FluxRelease{CRNamespace: "flux-system", CRName: "podinfo"}

	if cr.StorageKey() != "flux-system/podinfo" {
		t.Fatalf("key = %q", cr.StorageKey())
	}
}

func TestStorageKeyPrefersTheStorageNamespace(t *testing.T) {
	cr := FluxRelease{CRNamespace: "flux-system", CRName: "podinfo", StorageNamespace: "apps"}

	if cr.StorageKey() != "apps/podinfo" {
		t.Fatalf("key = %q, want the storage namespace", cr.StorageKey())
	}
}

func TestStorageKeyShortensANameOverFiftyThreeCharacters(t *testing.T) {
	long := strings.Repeat("a", 30)
	cr := FluxRelease{CRNamespace: "flux-system", CRName: long, TargetNamespace: strings.Repeat("b", 30)}

	composed := strings.Repeat("b", 30) + "-" + long
	sum := sha256.Sum256([]byte(composed))
	digest := hex.EncodeToString(sum[:])
	want := "flux-system/" + composed[:40] + "-" + digest[:12]
	if cr.StorageKey() != want {
		t.Fatalf("key = %q, want %q (first 40 chars + dash + first 12 hash chars)", cr.StorageKey(), want)
	}
}

func TestStorageKeyKeepsANameOfExactlyFiftyThreeCharacters(t *testing.T) {
	name := strings.Repeat("a", 53)
	cr := FluxRelease{CRNamespace: "flux-system", CRName: name}

	if cr.StorageKey() != "flux-system/"+name {
		t.Fatalf("key = %q, want the name untouched at the limit", cr.StorageKey())
	}
}

func TestOwnerIndexKeysEveryReleaseByItsStorageLocation(t *testing.T) {
	crs := []FluxRelease{
		{CRNamespace: "flux-system", CRName: "podinfo", Ref: api.ObjectRef{Name: "podinfo"}},
		{CRNamespace: "flux-system", CRName: "redis", StorageNamespace: "data", Ref: api.ObjectRef{Name: "redis"}},
	}

	index := OwnerIndex(crs)

	if index["flux-system/podinfo"].Name != "podinfo" {
		t.Fatalf("index = %v, want podinfo under its cr namespace", index)
	}
	if index["data/redis"].Name != "redis" {
		t.Fatalf("index = %v, want redis under its storage namespace", index)
	}
}
