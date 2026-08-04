package flux

import (
	"context"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
	"github.com/sophotechlabs/spinoza/internal/listerr"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

var groupOrder = []string{
	"Kustomizations",
	"Helm Releases",
	"Sources",
	"Image Automation",
	"Notifications",
}

var sourceResources = map[string]bool{
	"gitrepositories":  true,
	"helmrepositories": true,
	"ocirepositories":  true,
	"helmcharts":       true,
	"buckets":          true,
}

var imageResources = map[string]bool{
	"imagerepositories":      true,
	"imagepolicies":          true,
	"imageupdateautomations": true,
}

var notificationResources = map[string]bool{
	"alerts":    true,
	"providers": true,
	"receivers": true,
}

type Charts interface {
	Latest(repo charts.Repo, chart string) string
	Warm(repo charts.Repo, chart string)
}

const helmGroup = "helm.toolkit.fluxcd.io"

type Lister interface {
	List(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error)
	Warm(ctx context.Context, descs []api.ResourceDescriptor)
}

func Build(ctx context.Context, lister Lister, descs map[string]api.ResourceDescriptor, index Charts) api.FluxDashboard {
	byGroup := map[string][]api.FluxResource{}
	items := map[string][]*unstructured.Unstructured{}
	failures := listerr.New()
	lister.Warm(ctx, needed(descs, index))
	for _, desc := range descs {
		group := categoryOf(desc)
		if group == "" {
			continue
		}
		gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
		found, err := lister.List(ctx, desc)
		failures.Record(gvr.GroupResource().String(), err)
		if err != nil {
			continue
		}
		for _, item := range found {
			byGroup[group] = append(byGroup[group], resourceOf(item, desc))
			items[group] = append(items[group], item)
		}
	}
	applyLatest(byGroup, items, repos(ctx, lister, descs, index), index)
	dashboard := assemble(byGroup)
	dashboard.Error = failures.Message()
	return dashboard
}

func needed(descs map[string]api.ResourceDescriptor, index Charts) []api.ResourceDescriptor {
	out := []api.ResourceDescriptor{}
	for _, desc := range descs {
		if categoryOf(desc) != "" {
			out = append(out, desc)
			continue
		}
		if index != nil && isHelmRepository(desc) {
			out = append(out, desc)
		}
	}
	return out
}

func isHelmRepository(desc api.ResourceDescriptor) bool {
	if desc.Group != "source.toolkit.fluxcd.io" {
		return false
	}
	return desc.Resource == "helmrepositories"
}

func repos(ctx context.Context, lister Lister, descs map[string]api.ResourceDescriptor, index Charts) map[string]charts.Repo {
	if index == nil {
		return nil
	}
	return repoIndex(ctx, lister, descs)
}

func repoIndex(ctx context.Context, lister Lister, descs map[string]api.ResourceDescriptor) map[string]charts.Repo {
	out := map[string]charts.Repo{}
	for _, desc := range descs {
		if !isHelmRepository(desc) {
			continue
		}
		found, err := lister.List(ctx, desc)
		if err != nil {
			continue
		}
		for _, repo := range found {
			source := unstr.String(repo, "spec", "url")
			if charts.CheckRepoURL(source) != nil {
				continue
			}
			out[repo.GetNamespace()+"/"+repo.GetName()] = charts.Repo{
				URL: source,
				OCI: unstr.String(repo, "spec", "type") == "oci",
			}
		}
	}
	return out
}

func applyLatest(byGroup map[string][]api.FluxResource, items map[string][]*unstructured.Unstructured, repos map[string]charts.Repo, index Charts) {
	if index == nil {
		return
	}
	rows := byGroup["Helm Releases"]
	objects := items["Helm Releases"]
	for i := range rows {
		if i >= len(objects) {
			continue
		}
		repo, chart, ok := chartSource(objects[i], repos)
		if !ok {
			continue
		}
		index.Warm(repo, chart)
		latest := index.Latest(repo, chart)
		rows[i].Latest = latest
		rows[i].Outdated = charts.Newer(rows[i].Revision, latest)
	}
}

func chartSource(obj *unstructured.Unstructured, repos map[string]charts.Repo) (charts.Repo, string, bool) {
	chart := unstr.String(obj, "spec", "chart", "spec", "chart")
	if chart == "" {
		return charts.Repo{}, "", false
	}
	if unstr.String(obj, "spec", "chart", "spec", "sourceRef", "kind") != "HelmRepository" {
		return charts.Repo{}, "", false
	}
	name := unstr.String(obj, "spec", "chart", "spec", "sourceRef", "name")
	if name == "" {
		return charts.Repo{}, "", false
	}
	namespace := unstr.String(obj, "spec", "chart", "spec", "sourceRef", "namespace")
	if namespace == "" {
		namespace = obj.GetNamespace()
	}
	repo, ok := repos[namespace+"/"+name]
	if !ok {
		return charts.Repo{}, "", false
	}
	if repo.URL == "" {
		return charts.Repo{}, "", false
	}
	return repo, chart, true
}

func categoryOf(desc api.ResourceDescriptor) string {
	switch desc.Group {
	case "kustomize.toolkit.fluxcd.io":
		if desc.Resource == "kustomizations" {
			return "Kustomizations"
		}
	case helmGroup:
		if desc.Resource == "helmreleases" {
			return "Helm Releases"
		}
	case "source.toolkit.fluxcd.io":
		if sourceResources[desc.Resource] {
			return "Sources"
		}
	case "image.toolkit.fluxcd.io":
		if imageResources[desc.Resource] {
			return "Image Automation"
		}
	case "notification.toolkit.fluxcd.io":
		if notificationResources[desc.Resource] {
			return "Notifications"
		}
	default:
	}
	return ""
}

func assemble(byGroup map[string][]api.FluxResource) api.FluxDashboard {
	groups := make([]api.FluxGroup, 0, len(byGroup))
	for _, name := range groupOrder {
		items := byGroup[name]
		if len(items) == 0 {
			continue
		}
		sortResources(items)
		groups = append(groups, api.FluxGroup{
			Name:      name,
			Ready:     readyCount(items),
			Reporting: reportingCount(items),
			Total:     len(items),
			Resources: items,
		})
	}
	return api.FluxDashboard{Groups: groups}
}

func sortResources(items []api.FluxResource) {
	slices.SortFunc(items, func(left, right api.FluxResource) int {
		if left.Kind != right.Kind {
			return strings.Compare(left.Kind, right.Kind)
		}
		if left.Namespace != right.Namespace {
			return strings.Compare(left.Namespace, right.Namespace)
		}
		return strings.Compare(left.Name, right.Name)
	})
}

func readyCount(items []api.FluxResource) int {
	count := 0
	for _, it := range items {
		if it.Ready == "True" {
			count++
		}
	}
	return count
}

func reportingCount(items []api.FluxResource) int {
	count := 0
	for _, it := range items {
		if it.Ready != "" {
			count++
		}
	}
	return count
}

func resourceOf(obj *unstructured.Unstructured, desc api.ResourceDescriptor) api.FluxResource {
	ready, message := unstr.Ready(obj)
	return api.FluxResource{
		Kind:      desc.Kind,
		Group:     desc.Group,
		Version:   desc.Version,
		Resource:  desc.Resource,
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Ready:     ready,
		Suspended: unstr.Bool(obj, "spec", "suspend"),
		Revision:  revisionOf(obj, desc),
		Source:    sourceOf(obj),
		Message:   message,
		CreatedAt: obj.GetCreationTimestamp().Time.UTC().Format(time.RFC3339),
	}
}

func revisionOf(obj *unstructured.Unstructured, desc api.ResourceDescriptor) string {
	if desc.Group == helmGroup && desc.Resource == "helmreleases" {
		version := historyChartVersion(obj)
		if version != "" {
			return version
		}
	}
	paths := [][]string{
		{"status", "lastAppliedRevision"},
		{"status", "lastAttemptedRevision"},
		{"status", "artifact", "revision"},
		{"status", "latestImage"},
	}
	for _, p := range paths {
		v := unstr.String(obj, p...)
		if v != "" {
			return v
		}
	}
	return ""
}

func historyChartVersion(obj *unstructured.Unstructured) string {
	entries := unstr.Slice(obj, "status", "history")
	if len(entries) == 0 {
		return ""
	}
	entry, ok := entries[0].(map[string]any)
	if !ok {
		return ""
	}
	return unstr.At(entry, "chartVersion")
}

func sourceOf(obj *unstructured.Unstructured) string {
	kind, name := refAt(obj, "spec", "chartRef")
	if kind == "" || name == "" {
		kind, name = refAt(obj, "spec", "sourceRef")
	}
	if kind == "" || name == "" {
		kind, name = refAt(obj, "spec", "chart", "spec", "sourceRef")
	}
	if kind == "" || name == "" {
		return ""
	}
	return kind + "/" + name
}

func refAt(obj *unstructured.Unstructured, fields ...string) (kind, name string) {
	kind = unstr.String(obj, append(fields, "kind")...)
	name = unstr.String(obj, append(fields, "name")...)
	return kind, name
}
