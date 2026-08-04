package unstr

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func String(u *unstructured.Unstructured, fields ...string) string {
	value, found, err := unstructured.NestedString(u.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return value
}

func Bool(u *unstructured.Unstructured, fields ...string) bool {
	value, found, err := unstructured.NestedBool(u.Object, fields...)
	if !found || err != nil {
		return false
	}
	return value
}

func Int(u *unstructured.Unstructured, fields ...string) int64 {
	value, found, err := unstructured.NestedInt64(u.Object, fields...)
	if !found || err != nil {
		return 0
	}
	return value
}

func Slice(u *unstructured.Unstructured, fields ...string) []any {
	value, found, err := unstructured.NestedSlice(u.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return value
}

func Map(u *unstructured.Unstructured, fields ...string) (map[string]any, bool) {
	value, found, err := unstructured.NestedMap(u.Object, fields...)
	if !found || err != nil {
		return nil, false
	}
	return value, true
}

func At(entry map[string]any, key string) string {
	value, ok := entry[key].(string)
	if !ok {
		return ""
	}
	return value
}

func Ready(u *unstructured.Unstructured) (status, message string) {
	for _, raw := range Slice(u, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if At(entry, "type") != "Ready" {
			continue
		}
		return At(entry, "status"), At(entry, "message")
	}
	return "", ""
}

func ReadySummary(u *unstructured.Unstructured) string {
	for _, raw := range Slice(u, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if At(entry, "type") != "Ready" {
			continue
		}
		if At(entry, "status") == "True" {
			return "Ready"
		}
		reason := At(entry, "reason")
		if reason != "" {
			return reason
		}
		return "NotReady"
	}
	return ""
}
