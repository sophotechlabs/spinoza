package gitops

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

const maxDriftFields = 20

const noDeclaration = "no last-applied-configuration on this object, so there is nothing declared to compare; " +
	"ServerSideApply=true never writes one"

var ignoredPaths = map[string]bool{
	"metadata.creationTimestamp": true,
	"apiVersion":                 true,
	"kind":                       true,
	"metadata.name":              true,
	"metadata.namespace":         true,
	"status":                     true,
}

func Drift(live *unstructured.Unstructured) ([]api.FieldDrift, string) {
	declared, ok := declarationOf(live)
	if !ok {
		return nil, noDeclaration
	}
	found := []api.FieldDrift{}
	compare(declared, live.Object, "", &found)
	slices.SortFunc(found, func(left, right api.FieldDrift) int {
		return strings.Compare(left.Path, right.Path)
	})
	if len(found) <= maxDriftFields {
		return found, ""
	}
	return found[:maxDriftFields], fmt.Sprintf("%d more fields differ", len(found)-maxDriftFields)
}

func declarationOf(live *unstructured.Unstructured) (map[string]any, bool) {
	raw, carried := live.GetAnnotations()[lastAppliedAnnotation]
	if !carried || raw == "" {
		return nil, false
	}
	declared := map[string]any{}
	err := json.Unmarshal([]byte(raw), &declared)
	if err != nil {
		return nil, false
	}
	return declared, true
}

func compare(declared, live map[string]any, prefix string, out *[]api.FieldDrift) {
	for key, want := range declared {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if ignoredPaths[path] {
			continue
		}
		got, present := live[key]
		if !present {
			*out = append(*out, api.FieldDrift{Path: path, Declared: text(want), Live: "not set"})
			continue
		}
		wantMap, declaredIsMap := want.(map[string]any)
		gotMap, liveIsMap := got.(map[string]any)
		if declaredIsMap && liveIsMap {
			compare(wantMap, gotMap, path, out)
			continue
		}
		wantList, declaredIsList := want.([]any)
		gotList, liveIsList := got.([]any)
		if declaredIsList && liveIsList {
			compareLists(wantList, gotList, path, out)
			continue
		}
		if text(want) == text(got) {
			continue
		}
		*out = append(*out, api.FieldDrift{Path: path, Declared: text(want), Live: text(got)})
	}
}

func compareLists(declared, live []any, path string, out *[]api.FieldDrift) {
	if named(declared) {
		compareByName(declared, live, path, out)
		return
	}
	if len(declared) != len(live) {
		*out = append(*out, api.FieldDrift{Path: path, Declared: entries(len(declared)), Live: entries(len(live))})
		return
	}
	for at := range declared {
		compareEntry(declared[at], live[at], fmt.Sprintf("%s[%d]", path, at), out)
	}
}

func compareEntry(declared, live any, path string, out *[]api.FieldDrift) {
	wantMap, declaredIsMap := declared.(map[string]any)
	gotMap, liveIsMap := live.(map[string]any)
	if declaredIsMap && liveIsMap {
		compare(wantMap, gotMap, path, out)
		return
	}
	if text(declared) == text(live) {
		return
	}
	*out = append(*out, api.FieldDrift{Path: path, Declared: text(declared), Live: text(live)})
}

func named(entries []any) bool {
	if len(entries) == 0 {
		return false
	}
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		if nameOf(entry) == "" {
			return false
		}
	}
	return true
}

func nameOf(entry map[string]any) string {
	name, ok := entry["name"].(string)
	if !ok {
		return ""
	}
	return name
}

func compareByName(declared, live []any, path string, out *[]api.FieldDrift) {
	found := map[string]any{}
	for _, raw := range live {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		found[nameOf(entry)] = entry
	}
	for _, raw := range declared {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := nameOf(entry)
		here := fmt.Sprintf("%s[%s]", path, name)
		match, present := found[name]
		if !present {
			*out = append(*out, api.FieldDrift{Path: here, Declared: text(entry), Live: "not set"})
			continue
		}
		compareEntry(entry, match, here, out)
	}
}

func entries(count int) string {
	if count == 1 {
		return "1 entry"
	}
	return strconv.Itoa(count) + " entries"
}

func text(value any) string {
	switch typed := value.(type) {
	case nil:
		return "not set"
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}
