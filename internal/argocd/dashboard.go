package argocd

import (
	"context"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/listerr"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const Group = "argoproj.io"

const applications = "applications"

const applicationSets = "applicationsets"

const appProjects = "appprojects"

const trackingLabel = "app.kubernetes.io/instance"

const trackingAnnotation = "argocd.argoproj.io/tracking-id"

type Lister interface {
	List(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error)
	Warm(ctx context.Context, descs []api.ResourceDescriptor)
}

func Installed(descs map[string]api.ResourceDescriptor) bool {
	for _, desc := range descs {
		if desc.Group == Group && desc.Resource == applications {
			return true
		}
	}
	return false
}

func wanted(descs map[string]api.ResourceDescriptor) []api.ResourceDescriptor {
	out := []api.ResourceDescriptor{}
	for _, desc := range descs {
		if desc.Group != Group {
			continue
		}
		if !listed(desc.Resource) {
			continue
		}
		out = append(out, desc)
	}
	return out
}

func listed(resource string) bool {
	switch resource {
	case applications, applicationSets, appProjects:
		return true
	}
	return false
}

func Build(ctx context.Context, lister Lister, descs map[string]api.ResourceDescriptor) api.ArgoDashboard {
	types := wanted(descs)
	if len(types) == 0 {
		return api.ArgoDashboard{}
	}
	lister.Warm(ctx, types)
	failures := listerr.New()
	apps := []api.ArgoApp{}
	sets := []api.ArgoApp{}
	projects := []api.ArgoApp{}
	for _, desc := range types {
		gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
		found, err := lister.List(ctx, desc)
		failures.Record(gvr.GroupResource().String(), err)
		if err != nil {
			continue
		}
		for _, item := range found {
			switch desc.Resource {
			case applications:
				apps = append(apps, appOf(item, desc))
			case applicationSets:
				sets = append(sets, appOf(item, desc))
			default:
				projects = append(projects, appOf(item, desc))
			}
		}
	}
	slices.SortFunc(apps, byName)
	slices.SortFunc(sets, byName)
	slices.SortFunc(projects, byName)
	return api.ArgoDashboard{
		Apps:            withParents(apps, sets),
		ApplicationSets: sets,
		Projects:        projects,
		Error:           failures.Message(),
	}
}

func byName(left, right api.ArgoApp) int {
	if left.Namespace != right.Namespace {
		return strings.Compare(left.Namespace, right.Namespace)
	}
	return strings.Compare(left.Name, right.Name)
}

func appOf(item *unstructured.Unstructured, desc api.ResourceDescriptor) api.ArgoApp {
	return api.ArgoApp{
		Kind:        desc.Kind,
		Automation:  automationOf(item, desc),
		Group:       desc.Group,
		Version:     desc.Version,
		Resource:    desc.Resource,
		Name:        item.GetName(),
		Namespace:   item.GetNamespace(),
		Project:     unstr.String(item, "spec", "project"),
		Sync:        unstr.String(item, "status", "sync", "status"),
		Health:      unstr.String(item, "status", "health", "status"),
		Revision:    revisionOf(item),
		Repo:        sourceOf(item, "repoURL"),
		Path:        pathOf(item),
		Destination: destinationOf(item),
		Message:     messageOf(item),
		Owner:       ownerOf(item),
		CreatedAt:   item.GetCreationTimestamp().UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func automationOf(item *unstructured.Unstructured, desc api.ResourceDescriptor) string {
	if desc.Resource != applications {
		return ""
	}
	mode := syncModeOf(item)
	policy := policyOf(item)
	if policy == "" {
		return mode
	}
	return mode + " · " + policy
}

func revisionOf(item *unstructured.Unstructured) string {
	revision := unstr.String(item, "status", "sync", "revision")
	if revision != "" {
		return revision
	}
	return sourceOf(item, "targetRevision")
}

func sourceOf(item *unstructured.Unstructured, field string) string {
	single := unstr.String(item, "spec", "source", field)
	if single != "" {
		return single
	}
	sources := unstr.Slice(item, "spec", "sources")
	for _, raw := range sources {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value, ok := entry[field].(string)
		if ok && value != "" {
			return value
		}
	}
	return ""
}

func pathOf(item *unstructured.Unstructured) string {
	path := sourceOf(item, "path")
	if path != "" {
		return path
	}
	return sourceOf(item, "chart")
}

func destinationOf(item *unstructured.Unstructured) string {
	namespace := unstr.String(item, "spec", "destination", "namespace")
	server := unstr.String(item, "spec", "destination", "name")
	if server == "" {
		server = unstr.String(item, "spec", "destination", "server")
	}
	if namespace == "" {
		return server
	}
	if server == "" {
		return namespace
	}
	return server + " " + namespace
}

func messageOf(item *unstructured.Unstructured) string {
	message := unstr.String(item, "status", "health", "message")
	if message != "" {
		return message
	}
	return unstr.String(item, "status", "operationState", "message")
}

func ownerOf(item *unstructured.Unstructured) string {
	for _, owner := range item.GetOwnerReferences() {
		if owner.Kind == "ApplicationSet" {
			return owner.Name
		}
	}
	labels := item.GetLabels()
	tracked, ok := labels[trackingLabel]
	if ok && tracked != "" && tracked != item.GetName() {
		return tracked
	}
	return fromTrackingID(item.GetAnnotations()[trackingAnnotation], item.GetName())
}

func fromTrackingID(id, self string) string {
	if id == "" {
		return ""
	}
	owner, _, found := strings.Cut(id, ":")
	if !found || owner == self {
		return ""
	}
	return owner
}

func withParents(apps, sets []api.ArgoApp) []api.ArgoApp {
	known := map[string]bool{}
	for _, app := range apps {
		known[app.Name] = true
	}
	for _, set := range sets {
		known[set.Name] = true
	}
	out := make([]api.ArgoApp, 0, len(apps))
	for _, app := range apps {
		if !known[app.Owner] {
			app.Owner = ""
		}
		out = append(out, app)
	}
	return out
}
