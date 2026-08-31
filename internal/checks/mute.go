package checks

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const MutesKey = "spinoza.checks.muted.v1"

type Mute = api.Mute

func AllMutes(raw string) map[string][]Mute {
	if strings.TrimSpace(raw) == "" {
		return map[string][]Mute{}
	}
	var held map[string][]Mute
	if err := json.Unmarshal([]byte(raw), &held); err != nil {
		return map[string][]Mute{}
	}
	out := map[string][]Mute{}
	for cluster, list := range held {
		kept := make([]Mute, 0, len(list))
		for _, one := range list {
			if one.Check == "" {
				continue
			}
			kept = append(kept, one)
		}
		if len(kept) > 0 {
			out[cluster] = kept
		}
	}
	return out
}

func ParseMutes(raw, cluster string) []Mute {
	return AllMutes(raw)[cluster]
}

func EncodeMutes(all map[string][]Mute) string {
	body, err := json.Marshal(all)
	if err != nil {
		return ""
	}
	return string(body)
}

func RefKey(ref api.ObjectRef) string {
	return strings.Join([]string{ref.Group, ref.Version, ref.Resource, ref.Namespace, ref.Name}, "/")
}

const (
	ScopeObject     = "object"
	ScopeNamespace  = "namespace"
	ScopeCheck      = "check"
	ScopeConvention = "convention"
	ScopeRule       = "rule"
)

func silences(mute Mute, id string, item found) bool {
	if mute.Check != id {
		return false
	}
	if mute.Ref != "" {
		return mute.Ref == RefKey(item.subject.Ref)
	}
	if mute.Namespace != "" {
		return mute.Namespace == item.subject.Ref.Namespace
	}
	return true
}

func scopeOf(mute Mute) string {
	if mute.Ref != "" {
		return ScopeObject
	}
	if mute.Namespace != "" {
		return ScopeNamespace
	}
	return ScopeCheck
}

func SameMute(one, other Mute) bool {
	return one.Check == other.Check && one.Namespace == other.Namespace && one.Ref == other.Ref
}

func identityOf(id string, item found) string {
	ref := item.subject.Ref
	if generated := generatedName(item.subject); generated != "" {
		ref.Name = generated
	}
	return strings.Join([]string{id, RefKey(ref), item.container}, "\x00")
}

func labelOf(item found) string {
	parts := []string{item.subject.Kind, refLabel(item.subject.Ref)}
	if item.container != "" {
		parts = append(parts, "container "+item.container)
	}
	return strings.Join(parts, " ")
}

func refLabel(ref api.ObjectRef) string {
	if ref.Namespace == "" {
		return ref.Name
	}
	return ref.Namespace + "/" + ref.Name
}

func generatedName(subject Subject) string {
	if subject.Object == nil {
		return ""
	}
	return subject.Object.GetGenerateName()
}

const fingerprintBytes = 8

func fingerprintOf(key string) string {
	sum := sha256.Sum256([]byte(key))
	return base64.RawURLEncoding.EncodeToString(sum[:fingerprintBytes])
}
