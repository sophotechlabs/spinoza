package checks

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

const (
	ownerDepth = 4
	specField  = "spec"
)

type Lister interface {
	List(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error)
	Warm(ctx context.Context, descs []api.ResourceDescriptor)
	ListNames(ctx context.Context, desc api.ResourceDescriptor) ([]api.ObjectRef, error)
}

type target struct {
	group    string
	resource string
}

var targets = []target{
	{group: "", resource: "pods"},
	{group: "apps", resource: "deployments"},
	{group: "apps", resource: "statefulsets"},
	{group: "apps", resource: "daemonsets"},
	{group: "apps", resource: "replicasets"},
	{group: "", resource: "replicationcontrollers"},
	{group: "batch", resource: "jobs"},
	{group: "batch", resource: "cronjobs"},
}

var contextTargets = []target{
	{group: "", resource: "nodes"},
	{group: "", resource: "namespaces"},
	{group: "", resource: "resourcequotas"},
	{group: "", resource: "limitranges"},
	{group: "", resource: "services"},
	{group: "", resource: "serviceaccounts"},
	{group: "", resource: "configmaps"},
	{group: "", resource: "persistentvolumeclaims"},
	{group: "networking.k8s.io", resource: "ingresses"},
	{group: "networking.k8s.io", resource: "ingressclasses"},
	{group: "networking.k8s.io", resource: "networkpolicies"},
	{group: "policy", resource: "poddisruptionbudgets"},
	{group: "autoscaling", resource: "horizontalpodautoscalers"},
	{group: "storage.k8s.io", resource: "storageclasses"},
	{group: "scheduling.k8s.io", resource: "priorityclasses"},
	{group: "node.k8s.io", resource: "runtimeclasses"},
	{group: "rbac.authorization.k8s.io", resource: "roles"},
	{group: "rbac.authorization.k8s.io", resource: "rolebindings"},
	{group: "rbac.authorization.k8s.io", resource: "clusterroles"},
	{group: "rbac.authorization.k8s.io", resource: "clusterrolebindings"},
}

var nameOnlyTargets = []target{
	{group: "", resource: "secrets"},
}

func allTargets() []target {
	out := make([]target, 0, len(targets)+len(contextTargets)+len(nameOnlyTargets))
	out = append(out, targets...)
	out = append(out, contextTargets...)
	out = append(out, nameOnlyTargets...)
	return out
}

func isNameOnly(desc api.ResourceDescriptor) bool {
	for _, want := range nameOnlyTargets {
		if desc.Group == want.group && desc.Resource == want.resource {
			return true
		}
	}
	return false
}

type Container struct {
	Name string
	Init bool
	Spec map[string]any
}

type Placed struct {
	Name string
	Node string
}

type Subject struct {
	Ref        api.ObjectRef
	Kind       string
	Object     *unstructured.Unstructured
	Pod        map[string]any
	Containers []Container
	Replicas   int64
	Pods       []Placed
	Origin     string
	ManagedBy  string
}

type held struct {
	desc api.ResourceDescriptor
	obj  *unstructured.Unstructured
}

func needed(descs map[string]api.ResourceDescriptor) (found []api.ResourceDescriptor, absent []string) {
	wanted := allTargets()
	found = make([]api.ResourceDescriptor, 0, len(wanted))
	for _, want := range wanted {
		desc, ok := matching(descs, want)
		if !ok {
			absent = append(absent, want.resource)
			continue
		}
		found = append(found, desc)
	}
	return found, absent
}

func isSubjectKind(desc api.ResourceDescriptor) bool {
	for _, want := range targets {
		if desc.Group == want.group && desc.Resource == want.resource {
			return true
		}
	}
	return false
}

func matching(descs map[string]api.ResourceDescriptor, want target) (api.ResourceDescriptor, bool) {
	for _, desc := range descs {
		if desc.Group == want.group && desc.Resource == want.resource {
			return desc, true
		}
	}
	return api.ResourceDescriptor{}, false
}

func undiscovered(absent []string) string {
	if len(absent) == 0 {
		return ""
	}
	return "not discovered yet, so nothing of these types was audited: " + strings.Join(absent, ", ")
}

func gather(ctx context.Context, lister Lister, descs []api.ResourceDescriptor) ([]held, []api.ObjectRef, string) {
	warmed := []api.ResourceDescriptor{}
	for _, desc := range descs {
		if isNameOnly(desc) {
			continue
		}
		warmed = append(warmed, desc)
	}
	lister.Warm(ctx, warmed)
	failures := listerr.New()
	out := []held{}
	names := []api.ObjectRef{}
	for _, desc := range descs {
		gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
		if isNameOnly(desc) {
			found, err := lister.ListNames(ctx, desc)
			failures.Record(gvr.GroupResource().String(), err)
			if err == nil {
				names = append(names, found...)
			}
			continue
		}
		items, err := lister.List(ctx, desc)
		failures.Record(gvr.GroupResource().String(), err)
		if err != nil {
			continue
		}
		for _, item := range items {
			out = append(out, held{desc: desc, obj: item})
		}
	}
	return out, names, failures.Message()
}

func groupOf(apiVersion string) string {
	group, _, found := strings.Cut(apiVersion, "/")
	if !found {
		return ""
	}
	return group
}

func objectKey(obj *unstructured.Unstructured) string {
	return kindKey(obj.GroupVersionKind().Group, obj.GetKind()) +
		"/" + obj.GetNamespace() + "/" + obj.GetName()
}

func kindKey(group, kind string) string {
	return group + "/" + kind
}

func index(items []held) map[string]held {
	out := make(map[string]held, len(items))
	for _, item := range items {
		out[objectKey(item.obj)] = item
	}
	return out
}

func kindsOf(items []held) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		out[kindKey(item.obj.GroupVersionKind().Group, item.obj.GetKind())] = true
	}
	return out
}

func owned(obj *unstructured.Unstructured, kinds map[string]bool) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if kinds[kindKey(groupOf(ref.APIVersion), ref.Kind)] {
			return true
		}
	}
	return false
}

func ownerOf(item held, byKey map[string]held) (held, bool) {
	for _, ref := range item.obj.GetOwnerReferences() {
		key := kindKey(groupOf(ref.APIVersion), ref.Kind) +
			"/" + item.obj.GetNamespace() + "/" + ref.Name
		found, ok := byKey[key]
		if ok {
			return found, true
		}
	}
	return held{}, false
}

func topOwner(item held, byKey map[string]held) held {
	current := item
	for range ownerDepth {
		next, ok := ownerOf(current, byKey)
		if !ok {
			return current
		}
		current = next
	}
	return current
}

func placedByOwner(items []held, byKey map[string]held) map[string][]Placed {
	out := map[string][]Placed{}
	for _, item := range items {
		if item.obj.GetKind() != "Pod" {
			continue
		}
		key := objectKey(topOwner(item, byKey).obj)
		out[key] = append(out[key], Placed{
			Name: item.obj.GetName(),
			Node: unstr.String(item.obj, specField, "nodeName"),
		})
	}
	for key := range out {
		slices.SortFunc(out[key], func(left, right Placed) int {
			return strings.Compare(left.Name, right.Name)
		})
	}
	return out
}

func subjectsOf(items []held) []Subject {
	byKey := index(items)
	kinds := kindsOf(items)
	placed := placedByOwner(items, byKey)
	out := []Subject{}
	for _, item := range items {
		if !isSubjectKind(item.desc) || owned(item.obj, kinds) {
			continue
		}
		out = append(out, subjectOf(item, placed[objectKey(item.obj)]))
	}
	slices.SortFunc(out, func(left, right Subject) int {
		return strings.Compare(subjectKey(left), subjectKey(right))
	})
	return out
}

func subjectKey(subject Subject) string {
	return originRank(subject.Origin) + "\x00" +
		subject.Ref.Namespace + "/" + subject.Kind + "/" + subject.Ref.Name
}

func subjectOf(item held, placed []Placed) Subject {
	obj := item.obj
	kind := obj.GetKind()
	spec := specAt(obj, templatePath(kind)...)
	origin, managedBy := originOf(obj)
	return Subject{
		Ref: api.ObjectRef{
			Group:     item.desc.Group,
			Version:   item.desc.Version,
			Resource:  item.desc.Resource,
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		},
		Kind:       kind,
		Object:     obj,
		Pod:        spec,
		Containers: containersOf(spec),
		Replicas:   replicasOf(obj, kind),
		Pods:       placed,
		Origin:     origin,
		ManagedBy:  managedBy,
	}
}

func containersOf(spec map[string]any) []Container {
	out := listedContainers(spec, "initContainers", true)
	out = append(out, listedContainers(spec, "containers", false)...)
	slices.SortFunc(out, func(left, right Container) int {
		return strings.Compare(left.Name, right.Name)
	})
	return out
}

func listedContainers(spec map[string]any, field string, init bool) []Container {
	raw, ok := spec[field].([]any)
	if !ok {
		return nil
	}
	out := make([]Container, 0, len(raw))
	for _, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, Container{Name: unstr.At(item, "name"), Init: init, Spec: item})
	}
	return out
}

func replicasOf(obj *unstructured.Unstructured, kind string) int64 {
	switch kind {
	case "Deployment", "StatefulSet", "ReplicaSet", "ReplicationController":
		return countedOrOne(obj, specField, "replicas")
	case daemonSetKind:
		return unstr.Int(obj, "status", "desiredNumberScheduled")
	case "Job":
		return countedOrOne(obj, specField, "parallelism")
	default:
		return 1
	}
}

func countedOrOne(obj *unstructured.Unstructured, fields ...string) int64 {
	value, found, err := unstructured.NestedInt64(obj.Object, fields...)
	if !found || err != nil {
		return 1
	}
	return value
}

func specAt(obj *unstructured.Unstructured, fields ...string) map[string]any {
	value, found, err := unstructured.NestedFieldNoCopy(obj.Object, fields...)
	if !found || err != nil {
		return map[string]any{}
	}
	spec, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return spec
}

func templatePath(kind string) []string {
	if kind == "Pod" {
		return []string{specField}
	}
	if kind == "CronJob" {
		return []string{specField, "jobTemplate", specField, "template", specField}
	}
	return []string{specField, "template", specField}
}
