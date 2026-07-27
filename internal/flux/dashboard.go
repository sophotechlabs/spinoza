package flux

import (
	"context"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
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

func Build(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor, index Charts) api.FluxDashboard {
	byGroup := map[string][]api.FluxResource{}
	items := map[string][]*unstructured.Unstructured{}
	for _, d := range descs {
		group := categoryOf(d)
		if group == "" {
			continue
		}
		gvr := schema.GroupVersionResource{Group: d.Group, Version: d.Version, Resource: d.Resource}
		list, err := dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for i := range list.Items {
			byGroup[group] = append(byGroup[group], resourceOf(&list.Items[i], d))
			items[group] = append(items[group], &list.Items[i])
		}
	}
	applyLatest(byGroup, items, repoIndex(ctx, dyn, descs), index)
	return assemble(byGroup)
}

func repoIndex(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor) map[string]charts.Repo {
	out := map[string]charts.Repo{}
	for _, d := range descs {
		if d.Group != "source.toolkit.fluxcd.io" {
			continue
		}
		if d.Resource != "helmrepositories" {
			continue
		}
		gvr := schema.GroupVersionResource{Group: d.Group, Version: d.Version, Resource: d.Resource}
		list, err := dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for i := range list.Items {
			repo := &list.Items[i]
			out[repo.GetNamespace()+"/"+repo.GetName()] = charts.Repo{
				URL: nestedString(repo, "spec", "url"),
				OCI: nestedString(repo, "spec", "type") == "oci",
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

func chartSource(u *unstructured.Unstructured, repos map[string]charts.Repo) (charts.Repo, string, bool) {
	chart := nestedString(u, "spec", "chart", "spec", "chart")
	if chart == "" {
		return charts.Repo{}, "", false
	}
	if nestedString(u, "spec", "chart", "spec", "sourceRef", "kind") != "HelmRepository" {
		return charts.Repo{}, "", false
	}
	name := nestedString(u, "spec", "chart", "spec", "sourceRef", "name")
	if name == "" {
		return charts.Repo{}, "", false
	}
	namespace := nestedString(u, "spec", "chart", "spec", "sourceRef", "namespace")
	if namespace == "" {
		namespace = u.GetNamespace()
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

func categoryOf(d api.ResourceDescriptor) string {
	switch d.Group {
	case "kustomize.toolkit.fluxcd.io":
		if d.Resource == "kustomizations" {
			return "Kustomizations"
		}
	case "helm.toolkit.fluxcd.io":
		if d.Resource == "helmreleases" {
			return "Helm Releases"
		}
	case "source.toolkit.fluxcd.io":
		if sourceResources[d.Resource] {
			return "Sources"
		}
	case "image.toolkit.fluxcd.io":
		if imageResources[d.Resource] {
			return "Image Automation"
		}
	case "notification.toolkit.fluxcd.io":
		if notificationResources[d.Resource] {
			return "Notifications"
		}
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
			Total:     len(items),
			Resources: items,
		})
	}
	return api.FluxDashboard{Groups: groups}
}

func sortResources(items []api.FluxResource) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
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

func resourceOf(u *unstructured.Unstructured, d api.ResourceDescriptor) api.FluxResource {
	ready, message := readyCondition(u)
	return api.FluxResource{
		Kind:      d.Kind,
		Group:     d.Group,
		Version:   d.Version,
		Resource:  d.Resource,
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
		Ready:     ready,
		Suspended: nestedBool(u, "spec", "suspend"),
		Revision:  revisionOf(u),
		Source:    sourceOf(u),
		Message:   message,
		CreatedAt: u.GetCreationTimestamp().Time.UTC().Format(time.RFC3339),
	}
}

func readyCondition(u *unstructured.Unstructured) (status, message string) {
	for _, c := range nestedSlice(u, "status", "conditions") {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if m["type"] != "Ready" {
			continue
		}
		status, _ = m["status"].(string)
		message, _ = m["message"].(string)
		return status, message
	}
	return "", ""
}

func revisionOf(u *unstructured.Unstructured) string {
	paths := [][]string{
		{"status", "lastAppliedRevision"},
		{"status", "lastAttemptedRevision"},
		{"status", "artifact", "revision"},
		{"status", "latestImage"},
	}
	for _, p := range paths {
		v := nestedString(u, p...)
		if v != "" {
			return v
		}
	}
	return ""
}

func sourceOf(u *unstructured.Unstructured) string {
	kind := nestedString(u, "spec", "sourceRef", "kind")
	name := nestedString(u, "spec", "sourceRef", "name")
	if kind == "" || name == "" {
		kind = nestedString(u, "spec", "chart", "spec", "sourceRef", "kind")
		name = nestedString(u, "spec", "chart", "spec", "sourceRef", "name")
	}
	if kind == "" || name == "" {
		return ""
	}
	return kind + "/" + name
}

func nestedString(u *unstructured.Unstructured, fields ...string) string {
	v, found, err := unstructured.NestedString(u.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return v
}

func nestedBool(u *unstructured.Unstructured, fields ...string) bool {
	v, found, err := unstructured.NestedBool(u.Object, fields...)
	if !found || err != nil {
		return false
	}
	return v
}

func nestedSlice(u *unstructured.Unstructured, fields ...string) []interface{} {
	v, found, err := unstructured.NestedSlice(u.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return v
}
