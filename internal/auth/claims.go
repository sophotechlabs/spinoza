package auth

import (
	"slices"
	"strings"
)

func usernameFrom(claims claimSet, keys []string) string {
	for _, key := range keys {
		text, ok := claims[key].(string)
		if ok && text != "" {
			return text
		}
	}
	return ""
}

func groupsFrom(claims claimSet, key string) []string {
	held, present := claims[key]
	if !present {
		return nil
	}
	switch value := held.(type) {
	case string:
		return ParseList(value)
	case []string:
		return slices.Clone(value)
	case []any:
		return stringsIn(value)
	default:
		return nil
	}
}

func stringsIn(values []any) []string {
	out := make([]string, 0, len(values))
	for _, one := range values {
		text, ok := one.(string)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func prefixed(values []string, prefix string) []string {
	if prefix == "" {
		return values
	}
	out := make([]string, 0, len(values))
	for _, one := range values {
		out = append(out, prefix+one)
	}
	return out
}

func sessionIDFrom(claims claimSet) string {
	held, ok := claims["sid"].(string)
	if ok && held != "" {
		return held
	}
	return newSessionID()
}
