package helm

import (
	"cmp"
	"context"
	"errors"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var ErrNoRelease = errors.New("no such helm release")

type Kind struct {
	Group      string
	Version    string
	Resource   string
	Namespaced bool
}

type Resolver func(apiVersion, kind string) (Kind, bool)

func (s *Service) Detail(
	ctx context.Context,
	namespace, name string,
	revision int64,
	resolve Resolver,
) (api.HelmReleaseDetail, error) {
	if !nameFormat.MatchString(namespace) || !nameFormat.MatchString(name) {
		return api.HelmReleaseDetail{}, errors.New("namespace and name must be valid kubernetes names")
	}
	bounded, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	var ref storedRef
	var err error
	if revision < 1 {
		ref, err = s.latestRef(bounded, namespace, name)
	} else {
		ref, err = s.refAt(bounded, namespace, name, revision)
	}
	if err != nil {
		return api.HelmReleaseDetail{}, err
	}
	body, bodyErr := s.body(bounded, ref)
	if bodyErr != nil {
		return api.HelmReleaseDetail{}, bodyErr
	}
	selected := stored{
		driver:    ref.driver,
		namespace: ref.namespace,
		name:      ref.name,
		revision:  ref.revision,
		status:    ref.status,
		created:   ref.created,
		body:      body,
	}
	release, decoded, decodeErr := selected.decoded()
	detail := api.HelmReleaseDetail{
		Release: release,
		Driver:  selected.driver,
		History: []api.HelmRevision{},
	}
	if decodeErr != nil {
		detail.Error = "the selected revision's payload could not be read: " + decodeErr.Error()
		return detail, nil
	}

	detail.Values = valuesOf(decoded)
	detail.Notes = decoded.Info.Notes
	detail.Manifest = decoded.Manifest
	detail.FirstDeployed = decoded.Info.FirstDeployed
	detail.Resources = resourcesOf(decoded.Manifest, release.Namespace, resolve)
	return detail, nil
}

func newerRevision(left, right stored) int {
	return cmp.Compare(right.revision, left.revision)
}

func valuesOf(decoded payload) string {
	if len(decoded.Config) == 0 {
		return ""
	}
	body, err := yaml.Marshal(decoded.Config)
	if err != nil {
		return ""
	}
	return string(body)
}

type manifestDoc struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
}

func resourcesOf(manifest, namespace string, resolve Resolver) []api.HelmResource {
	out := []api.HelmResource{}
	for doc := range strings.SplitSeq(manifest, "\n---") {
		trimmed := strings.TrimSpace(doc)
		if trimmed == "" {
			continue
		}
		var parsed manifestDoc
		err := yaml.Unmarshal([]byte(trimmed), &parsed)
		if err != nil {
			continue
		}
		if parsed.Kind == "" || parsed.Metadata.Name == "" {
			continue
		}
		out = append(out, resourceOf(parsed, namespace, resolve))
	}
	return out
}

func resourceOf(parsed manifestDoc, namespace string, resolve Resolver) api.HelmResource {
	resource := api.HelmResource{
		APIVersion: parsed.APIVersion,
		Kind:       parsed.Kind,
		Name:       parsed.Metadata.Name,
		Namespace:  parsed.Metadata.Namespace,
	}
	if resolve == nil {
		return resource
	}
	found, ok := resolve(parsed.APIVersion, parsed.Kind)
	if !ok {
		return resource
	}
	resource.Group = found.Group
	resource.Version = found.Version
	resource.Resource = found.Resource
	if found.Namespaced && resource.Namespace == "" {
		resource.Namespace = namespace
	}
	return resource
}
