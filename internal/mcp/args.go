package mcp

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	argResource  = "resource"
	argName      = "name"
	argNamespace = "namespace"
	argGroup     = "group"
	argLimit     = "limit"
	argQuery     = "query"
	argAction    = "action"
	argYAML      = "yaml"
	keyError     = "error"
)

type arguments map[string]any

func (a arguments) text(key string) string {
	raw, held := a[key]
	if !held {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
}

func (a arguments) required(key string) (string, error) {
	found := a.text(key)
	if strings.TrimSpace(found) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return found, nil
}

func (a arguments) number(key string, fallback int) int {
	raw, held := a[key]
	if !held {
		return fallback
	}
	switch value := raw.(type) {
	case float64:
		return int(value)
	case string:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}

func (a arguments) flag(key string) bool {
	raw, held := a[key]
	if !held {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return value == "true"
	default:
		return false
	}
}

func (a arguments) oneOf(key string, allowed ...string) (string, error) {
	found, err := a.required(key)
	if err != nil {
		return "", err
	}
	if slices.Contains(allowed, found) {
		return found, nil
	}
	return "", fmt.Errorf("%s must be one of %s", key, strings.Join(allowed, ", "))
}

func (a arguments) ref(catalog api.ResourceCatalog) (api.ObjectRef, error) {
	resource, err := a.required(argResource)
	if err != nil {
		return api.ObjectRef{}, err
	}
	name, err := a.required(argName)
	if err != nil {
		return api.ObjectRef{}, err
	}
	found, err := resolve(catalog, resource, a.text(argGroup))
	if err != nil {
		return api.ObjectRef{}, err
	}
	return api.ObjectRef{
		Group:     found.Group,
		Version:   found.Version,
		Resource:  found.Resource,
		Namespace: a.text(argNamespace),
		Name:      name,
	}, nil
}

func (a arguments) kind(catalog api.ResourceCatalog) (api.ObjectRef, error) {
	resource, err := a.required(argResource)
	if err != nil {
		return api.ObjectRef{}, err
	}
	found, err := resolve(catalog, resource, a.text(argGroup))
	if err != nil {
		return api.ObjectRef{}, err
	}
	return api.ObjectRef{
		Group:     found.Group,
		Version:   found.Version,
		Resource:  found.Resource,
		Namespace: a.text(argNamespace),
	}, nil
}

func resolve(catalog api.ResourceCatalog, resource, group string) (api.ResourceDescriptor, error) {
	matches := []api.ResourceDescriptor{}
	wanted := strings.ToLower(resource)
	for _, category := range catalog.Categories {
		for _, desc := range category.Resources {
			if !names(desc, wanted) {
				continue
			}
			if group != "" && desc.Group != group {
				continue
			}
			matches = append(matches, desc)
		}
	}
	if len(matches) == 0 {
		return api.ResourceDescriptor{}, fmt.Errorf("this cluster reports no resource type called %q", resource)
	}
	if len(matches) > 1 {
		return api.ResourceDescriptor{}, fmt.Errorf("%q is served by %s; name the group", resource, groupList(matches))
	}
	return matches[0], nil
}

func names(desc api.ResourceDescriptor, wanted string) bool {
	if strings.EqualFold(desc.Resource, wanted) {
		return true
	}
	return strings.EqualFold(desc.Kind, wanted)
}

func groupList(matches []api.ResourceDescriptor) string {
	out := make([]string, 0, len(matches))
	for _, desc := range matches {
		label := desc.Group
		if label == "" {
			label = "the core group"
		}
		out = append(out, label)
	}
	return strings.Join(out, " and ")
}
