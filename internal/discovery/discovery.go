package discovery

import (
	"cmp"
	"slices"
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
			resource := list.APIResources[i]
			if strings.Contains(resource.Name, "/") {
				continue
			}
			if !supportsListWatch(resource.Verbs) {
				continue
			}
			desc := api.ResourceDescriptor{
				Group:      gv.Group,
				Version:    gv.Version,
				Resource:   resource.Name,
				Kind:       resource.Kind,
				Namespaced: resource.Namespaced,
				Category:   categoryFor(gv.Group, resource.Name),
			}
			descs = append(descs, desc)
			byKey[Key(gv.Group, gv.Version, resource.Name)] = desc
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
		sortDescs(name, rs)
		cats = append(cats, api.Category{Name: name, Resources: rs})
		seen[name] = true
	}
	extra := []string{}
	for name := range buckets {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	slices.Sort(extra)
	for _, name := range extra {
		rs := buckets[name]
		sortDescs(name, rs)
		cats = append(cats, api.Category{Name: name, Resources: rs})
	}
	return cats
}

var kindOrder = map[string][]string{
	"Cluster": {"Node", "Namespace", "Event"},
	"Workloads": {
		"Pod", "Deployment", "DaemonSet", "StatefulSet",
		"ReplicaSet", "ReplicationController", "Job", "CronJob",
	},
	"Config": {
		"ConfigMap", "Secret", "ResourceQuota", "LimitRange",
		"HorizontalPodAutoscaler", "PodDisruptionBudget", "PriorityClass",
		"RuntimeClass", "Lease",
	},
	"Network": {"Service", "Endpoints", "EndpointSlice", "Ingress", "IngressClass", "NetworkPolicy"},
	"Storage": {"PersistentVolumeClaim", "PersistentVolume", "StorageClass"},
	"Access Control": {
		"ServiceAccount", "Role", "RoleBinding", "ClusterRole", "ClusterRoleBinding",
	},
}

func rankOf(category, kind string) int {
	for i, name := range kindOrder[category] {
		if name == kind {
			return i
		}
	}
	return len(kindOrder[category])
}

func sortDescs(category string, ds []api.ResourceDescriptor) {
	slices.SortFunc(ds, func(left, right api.ResourceDescriptor) int {
		leftRank := rankOf(category, left.Kind)
		rightRank := rankOf(category, right.Kind)
		if leftRank != rightRank {
			return cmp.Compare(leftRank, rightRank)
		}
		return strings.Compare(left.Resource, right.Resource)
	})
}
