package gitops

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const ownershipNote = "this object is applied server-side, so there is no declared copy to diff; " +
	"these are the fields another writer has taken over"

const heldNote = "no spec field is held by anything other than "

var gitopsManagers = map[string][]string{
	api.ControllerArgo: {"argocd-controller", "argocd-application-controller"},
	api.ControllerFlux: {"kustomize-controller", "helm-controller"},
}

func Ownership(live *unstructured.Unstructured, controller string) ([]api.FieldDrift, string) {
	entries := live.GetManagedFields()
	owner, found := gitopsOwner(entries, controller)
	if !found {
		return nil, noDeclaration
	}
	taken := takenFrom(entries, owner)
	if len(taken) == 0 {
		return nil, heldNote + owner
	}
	slices.SortFunc(taken, func(left, right api.FieldDrift) int {
		return strings.Compare(left.Path, right.Path)
	})
	if len(taken) <= maxDriftFields {
		return taken, ownershipNote
	}
	return taken[:maxDriftFields], fmt.Sprintf("%s, and %d more", ownershipNote, len(taken)-maxDriftFields)
}

func gitopsOwner(entries []metav1.ManagedFieldsEntry, controller string) (string, bool) {
	wanted := gitopsManagers[controller]
	for i := range entries {
		entry := &entries[i]
		if !slices.Contains(wanted, entry.Manager) {
			continue
		}
		if entry.Subresource != "" {
			continue
		}
		return entry.Manager, true
	}
	return "", false
}

func takenFrom(entries []metav1.ManagedFieldsEntry, owner string) []api.FieldDrift {
	out := []api.FieldDrift{}
	seen := map[string]bool{}
	for i := range entries {
		entry := &entries[i]
		if entry.Manager == owner || entry.Subresource != "" {
			continue
		}
		for _, path := range sorted(specPathsOf(entry)) {
			if seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, api.FieldDrift{Path: path, Declared: owner, Live: entry.Manager})
		}
	}
	return out
}

func sorted(paths map[string]bool) []string {
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}

func specPathsOf(entry *metav1.ManagedFieldsEntry) map[string]bool {
	out := map[string]bool{}
	if entry.FieldsV1 == nil {
		return out
	}
	fields := map[string]any{}
	err := json.Unmarshal(entry.FieldsV1.GetRawBytes(), &fields)
	if err != nil {
		return out
	}
	spec, ok := fields["f:spec"].(map[string]any)
	if !ok {
		return out
	}
	walkFields(spec, "spec", out)
	return out
}

func walkFields(fields map[string]any, prefix string, out map[string]bool) {
	if len(fields) == 0 {
		out[prefix] = true
		return
	}
	for key, raw := range fields {
		path := prefix + stepOf(key)
		nested, ok := raw.(map[string]any)
		if !ok {
			out[path] = true
			continue
		}
		walkFields(nested, path, out)
	}
}

func stepOf(key string) string {
	name, found := strings.CutPrefix(key, "f:")
	if found {
		return "." + name
	}
	item, found := strings.CutPrefix(key, "k:")
	if found {
		return "[" + keyName(item) + "]"
	}
	value, found := strings.CutPrefix(key, "v:")
	if found {
		return "[" + strings.Trim(value, `"`) + "]"
	}
	index, found := strings.CutPrefix(key, "i:")
	if found {
		return "[" + index + "]"
	}
	return "." + key
}

func keyName(raw string) string {
	entry := map[string]any{}
	err := json.Unmarshal([]byte(raw), &entry)
	if err != nil {
		return raw
	}
	name, ok := entry["name"].(string)
	if ok {
		return name
	}
	return compact(entry)
}

func compact(entry map[string]any) string {
	parts := make([]string, 0, len(entry))
	for key, value := range entry {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}
