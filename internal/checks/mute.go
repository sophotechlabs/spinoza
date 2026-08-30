package checks

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// MutesKey is where the settings store holds what you have decided about.
// Mutes are kept per cluster: a judgement about one cluster's workload has no
// business following you to another.
const MutesKey = "spinoza.checks.muted.v1"

// Mute silences one check, in one namespace, or on one object, and keeps the
// reason with it. An empty Ref and Namespace means the check everywhere.
type Mute = api.Mute

// AllMutes reads what the settings store holds, which is a cluster to mute-list
// object. Anything unreadable yields no mutes rather than an error: an audit is
// not the place to argue about the settings file.
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

// RefKey names one object the way a mute and a baseline both have to name it.
// Neither the origin ranking nor the finding's wording belongs in here: a
// workload moves between yours and packaged when its labels change, and the
// wording of a check is edited, and a mute must survive both.
func RefKey(ref api.ObjectRef) string {
	return strings.Join([]string{ref.Group, ref.Version, ref.Resource, ref.Namespace, ref.Name}, "/")
}

// Scopes are what a mute was made at, so undoing one from a finding removes the
// mute that silenced it rather than a narrower one nobody made.
const (
	ScopeObject    = "object"
	ScopeNamespace = "namespace"
	ScopeCheck     = "check"
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

// SameMute identifies a mute for removal. Two mutes are the same when they
// silence the same thing, whatever reason each was given.
func SameMute(one, other Mute) bool {
	return one.Check == other.Check && one.Namespace == other.Namespace && one.Ref == other.Ref
}

// identityOf is what a baseline remembers a finding by. A pod whose name the
// apiserver generated is a different object every time it is replaced, so the
// generateName stands in for it: the workload is the same one, and a rollout is
// not a hundred new findings.
func identityOf(id string, item found) string {
	ref := item.subject.Ref
	if generated := generatedName(item.subject); generated != "" {
		ref.Name = generated
	}
	return strings.Join([]string{id, RefKey(ref), item.container}, "\x00")
}

func generatedName(subject Subject) string {
	if subject.Object == nil {
		return ""
	}
	return subject.Object.GetGenerateName()
}

const fingerprintBytes = 8

// Fingerprints are what a baseline stores instead of the findings themselves:
// they answer "was this here last time" without putting the name of every
// workload on disk.
func fingerprintOf(key string) string {
	sum := sha256.Sum256([]byte(key))
	return base64.RawURLEncoding.EncodeToString(sum[:fingerprintBytes])
}
