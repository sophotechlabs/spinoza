package helm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
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

func (s *Service) Detail(ctx context.Context, namespace, name string, resolve Resolver) (api.HelmReleaseDetail, error) {
	if !nameFormat.MatchString(namespace) || !nameFormat.MatchString(name) {
		return api.HelmReleaseDetail{}, errors.New("namespace and name must be valid kubernetes names")
	}
	bounded, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	revisions, err := revisionsIn(bounded, s.cs, namespace, name)
	if err != nil {
		return api.HelmReleaseDetail{}, err
	}
	if len(revisions) == 0 {
		return api.HelmReleaseDetail{}, fmt.Errorf("%w: %s/%s", ErrNoRelease, namespace, name)
	}
	slices.SortFunc(revisions, newerRevision)

	newest := revisions[0]
	release, decodeErr := newest.release()
	detail := api.HelmReleaseDetail{
		Release: release,
		Driver:  newest.driver,
		History: historyOf(revisions),
	}
	if decodeErr != nil {
		detail.Error = "the newest revision's payload could not be read: " + decodeErr.Error()
		return detail, nil
	}

	decoded, _ := decode(newest.body)
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

func historyOf(revisions []stored) []api.HelmRevision {
	out := make([]api.HelmRevision, 0, len(revisions))
	for _, item := range revisions {
		release, err := item.release()
		entry := api.HelmRevision{
			Revision:     release.Revision,
			Status:       release.Status,
			ChartVersion: release.ChartVersion,
			AppVersion:   release.AppVersion,
			Updated:      release.Updated,
			Description:  release.Description,
		}
		if err != nil {
			entry.Description = "this revision's payload could not be read"
		}
		out = append(out, entry)
	}
	return out
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
