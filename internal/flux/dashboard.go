package flux

import (
	"context"
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
	"github.com/sophotechlabs/spinoza/internal/listerr"
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
	failures := listerr.New()
	for _, desc := range descs {
		group := categoryOf(desc)
		if group == "" {
			continue
		}
		gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
		list, err := dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
		failures.Record(gvr.GroupResource().String(), err)
		if err != nil {
			continue
		}
		for i := range list.Items {
			byGroup[group] = append(byGroup[group], resourceOf(&list.Items[i], desc))
			items[group] = append(items[group], &list.Items[i])
		}
	}
	applyLatest(byGroup, items, repoIndex(ctx, dyn, descs), index)
	dashboard := assemble(byGroup)
	dashboard.Error = failures.Message()
	return dashboard
}

func repoIndex(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor) map[string]charts.Repo {
	out := map[string]charts.Repo{}
	for _, desc := range descs {
		if desc.Group != "source.toolkit.fluxcd.io" {
			continue
		}
		if desc.Resource != "helmrepositories" {
			continue
		}
		gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
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

func chartSource(obj *unstructured.Unstructured, repos map[string]charts.Repo) (charts.Repo, string, bool) {
	chart := nestedString(obj, "spec", "chart", "spec", "chart")
	if chart == "" {
		return charts.Repo{}, "", false
	}
	if nestedString(obj, "spec", "chart", "spec", "sourceRef", "kind") != "HelmRepository" {
		return charts.Repo{}, "", false
	}
	name := nestedString(obj, "spec", "chart", "spec", "sourceRef", "name")
	if name == "" {
		return charts.Repo{}, "", false
	}
	namespace := nestedString(obj, "spec", "chart", "spec", "sourceRef", "namespace")
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
	case "helm.toolkit.fluxcd.io":
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
	ready, message := readyCondition(obj)
	return api.FluxResource{
		Kind:      desc.Kind,
		Group:     desc.Group,
		Version:   desc.Version,
		Resource:  desc.Resource,
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Ready:     ready,
		Suspended: nestedBool(obj, "spec", "suspend"),
		Revision:  revisionOf(obj),
		Source:    sourceOf(obj),
		Message:   message,
		CreatedAt: obj.GetCreationTimestamp().Time.UTC().Format(time.RFC3339),
	}
}

func readyCondition(u *unstructured.Unstructured) (status, message string) {
	for _, c := range nestedSlice(u, "status", "conditions") {
		entry, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if entry["type"] != "Ready" {
			continue
		}
		status = stringAt(entry, "status")
		message = stringAt(entry, "message")
		return status, message
	}
	return "", ""
}

func revisionOf(obj *unstructured.Unstructured) string {
	paths := [][]string{
		{"status", "lastAppliedRevision"},
		{"status", "lastAttemptedRevision"},
		{"status", "artifact", "revision"},
		{"status", "latestImage"},
	}
	for _, p := range paths {
		v := nestedString(obj, p...)
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

func nestedSlice(u *unstructured.Unstructured, fields ...string) []any {
	v, found, err := unstructured.NestedSlice(u.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return v
}

func stringAt(m map[string]any, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}
