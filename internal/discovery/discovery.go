package discovery

import (
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func Key(group, version, resource string) string {
	return group + "/" + version + "/" + resource
}

func List(client discovery.DiscoveryInterface) ([]api.Category, map[string]api.ResourceDescriptor, error) {
	lists, err := client.ServerPreferredResources()
	descs := []api.ResourceDescriptor{}
	byKey := map[string]api.ResourceDescriptor{}
	for _, list := range lists {
		if list == nil {
			continue
		}
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		for i := range list.APIResources {
			r := list.APIResources[i]
			if strings.Contains(r.Name, "/") {
				continue
			}
			if !supportsListWatch(r.Verbs) {
				continue
			}
			d := api.ResourceDescriptor{
				Group:      gv.Group,
				Version:    gv.Version,
				Resource:   r.Name,
				Kind:       r.Kind,
				Namespaced: r.Namespaced,
				Category:   categoryFor(gv.Group, r.Name),
			}
			descs = append(descs, d)
			byKey[Key(gv.Group, gv.Version, r.Name)] = d
		}
	}
	return groupByCategory(descs), byKey, err
}

func supportsListWatch(verbs metav1.Verbs) bool {
	hasList := false
	hasWatch := false
	for _, v := range verbs {
		if v == "list" {
			hasList = true
		}
		if v == "watch" {
			hasWatch = true
		}
	}
	return hasList && hasWatch
}

func categoryFor(group, resource string) string {
	switch group {
	case "apps", "batch":
		return "Workloads"
	case "networking.k8s.io", "discovery.k8s.io":
		return "Network"
	case "storage.k8s.io":
		return "Storage"
	case "rbac.authorization.k8s.io":
		return "Access Control"
	case "autoscaling", "policy", "scheduling.k8s.io", "node.k8s.io", "coordination.k8s.io":
		return "Config"
	case "":
		return coreCategoryFor(resource)
	case "apiextensions.k8s.io", "apiregistration.k8s.io", "admissionregistration.k8s.io", "certificates.k8s.io", "authentication.k8s.io", "authorization.k8s.io", "flowcontrol.apiserver.k8s.io", "events.k8s.io":
		return "Cluster"
	default:
		return "Custom Resources"
	}
}

func coreCategoryFor(resource string) string {
	switch resource {
	case "pods", "replicationcontrollers":
		return "Workloads"
	case "configmaps", "secrets", "resourcequotas", "limitranges":
		return "Config"
	case "services", "endpoints":
		return "Network"
	case "persistentvolumeclaims", "persistentvolumes":
		return "Storage"
	case "serviceaccounts":
		return "Access Control"
	default:
		return "Cluster"
	}
}

func groupByCategory(descs []api.ResourceDescriptor) []api.Category {
	order := []string{"Cluster", "Workloads", "Config", "Network", "Storage", "Access Control", "Custom Resources"}
	buckets := map[string][]api.ResourceDescriptor{}
	for _, d := range descs {
		buckets[d.Category] = append(buckets[d.Category], d)
	}
	cats := []api.Category{}
	seen := map[string]bool{}
	for _, name := range order {
		rs, ok := buckets[name]
		if !ok {
			continue
		}
		sortDescs(rs)
		cats = append(cats, api.Category{Name: name, Resources: rs})
		seen[name] = true
	}
	extra := []string{}
	for name := range buckets {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		rs := buckets[name]
		sortDescs(rs)
		cats = append(cats, api.Category{Name: name, Resources: rs})
	}
	return cats
}

func sortDescs(ds []api.ResourceDescriptor) {
	sort.Slice(ds, func(i, j int) bool {
		return ds[i].Resource < ds[j].Resource
	})
}
