package helm

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	FluxGroup        = "helm.toolkit.fluxcd.io"
	FluxResource     = "helmreleases"
	maxReleaseName   = 53
	shortenedKeep    = 40
	shortenedHashLen = 12
)

type FluxRelease struct {
	CRNamespace      string
	CRName           string
	ReleaseName      string
	TargetNamespace  string
	StorageNamespace string
	Ref              api.ObjectRef
}

func (f FluxRelease) StorageKey() string {
	return f.storageNamespace() + "/" + f.releaseName()
}

func (f FluxRelease) releaseName() string {
	if f.ReleaseName != "" {
		return f.ReleaseName
	}
	composed := f.CRName
	if f.TargetNamespace != "" {
		composed = f.TargetNamespace + "-" + f.CRName
	}
	return shortenName(composed)
}

func (f FluxRelease) storageNamespace() string {
	if f.StorageNamespace != "" {
		return f.StorageNamespace
	}
	return f.CRNamespace
}

func shortenName(name string) string {
	if len(name) <= maxReleaseName {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	digest := hex.EncodeToString(sum[:])
	return name[:shortenedKeep] + "-" + digest[:shortenedHashLen]
}

func OwnerIndex(crs []FluxRelease) map[string]api.ObjectRef {
	out := map[string]api.ObjectRef{}
	for _, cr := range crs {
		out[cr.StorageKey()] = cr.Ref
	}
	return out
}
