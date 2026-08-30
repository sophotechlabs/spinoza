package checks

import (
	"context"
	"slices"
	"strings"
	"sync"

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
	// Scan reads a kind straight from the apiserver, with no cache and no
	// watch behind it. The custom resources are read this way: they are here
	// only to say which names something in the cluster mentions, and holding
	// every one of them in a cache costs more than a gigabyte on a large one.
	Scan(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error)
	Warm(ctx context.Context, descs []api.ResourceDescriptor)
	ListNames(ctx context.Context, desc api.ResourceDescriptor) ([]api.ObjectRef, error)
	Cached() []api.ResourceDescriptor
	Facts() Facts
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

var factTargets = []target{
	{group: "", resource: "nodes"},
	{group: "", resource: "namespaces"},
	{group: "", resource: "resourcequotas"},
	{group: "", resource: "limitranges"},
}

var contextTargets = []target{
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
	return targetsFor(true)
}

func targetsFor(wholeCluster bool) []target {
	out := make([]target, 0, len(targets)+len(contextTargets)+len(nameOnlyTargets))
	out = append(out, targets...)
	out = append(out, factTargets...)
	if !wholeCluster {
		return out
	}
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

func needed(descs map[string]api.ResourceDescriptor, wholeCluster bool) (found []api.ResourceDescriptor, absent []string) {
	wanted := targetsFor(wholeCluster)
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

// customResources is the category discovery gives anything outside the
// Kubernetes API groups. Reading them is what lets "nothing references this"
// be true on a cluster where cert-manager, Flux and Cilium name half the
// secrets in it.
const customResources = "Custom Resources"

func customKinds(descs map[string]api.ResourceDescriptor) []api.ResourceDescriptor {
	out := []api.ResourceDescriptor{}
	for _, desc := range descs {
		if desc.Category != customResources {
			continue
		}
		out = append(out, desc)
	}
	slices.SortFunc(out, func(left, right api.ResourceDescriptor) int {
		return strings.Compare(descKey(left), descKey(right))
	})
	return out
}

const extraWarmKinds = 25

func alsoWarm(lister Lister, already []api.ResourceDescriptor) []api.ResourceDescriptor {
	seen := map[string]bool{}
	for _, desc := range already {
		seen[descKey(desc)] = true
	}
	out := []api.ResourceDescriptor{}
	for _, desc := range lister.Cached() {
		if seen[descKey(desc)] {
			continue
		}
		seen[descKey(desc)] = true
		out = append(out, desc)
	}
	slices.SortFunc(out, func(left, right api.ResourceDescriptor) int {
		return strings.Compare(descKey(left), descKey(right))
	})
	if len(out) > extraWarmKinds {
		return out[:extraWarmKinds]
	}
	return out
}

func descKey(desc api.ResourceDescriptor) string {
	return desc.Group + "/" + desc.Version + "/" + desc.Resource
}

func everyDiscovered(descs map[string]api.ResourceDescriptor) []api.ResourceDescriptor {
	out := make([]api.ResourceDescriptor, 0, len(descs))
	for _, desc := range descs {
		out = append(out, desc)
	}
	slices.SortFunc(out, func(left, right api.ResourceDescriptor) int {
		return strings.Compare(descKey(left), descKey(right))
	})
	return out
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

func gather(
	ctx context.Context, lister Lister, descs []api.ResourceDescriptor,
) ([]held, []api.ObjectRef, []target, map[string]int, string) {
	warmed := []api.ResourceDescriptor{}
	for _, desc := range descs {
		if isNameOnly(desc) || borrowed(desc) {
			continue
		}
		warmed = append(warmed, desc)
	}
	lister.Warm(ctx, warmed)
	mentions, refused := scanMentions(ctx, lister, descs)
	failures := listerr.New()
	out := []held{}
	names := []api.ObjectRef{}
	unread := refused
	for _, desc := range descs {
		gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
		if borrowed(desc) {
			failures.Record(gvr.GroupResource().String(), mentions.errs[descKey(desc)])
			continue
		}
		if isNameOnly(desc) {
			found, err := lister.ListNames(ctx, desc)
			failures.Record(gvr.GroupResource().String(), err)
			if err != nil {
				unread = append(unread, target{group: desc.Group, resource: desc.Resource})
				continue
			}
			names = append(names, found...)
			continue
		}
		items, err := lister.List(ctx, desc)
		failures.Record(gvr.GroupResource().String(), err)
		if err != nil {
			unread = append(unread, target{group: desc.Group, resource: desc.Resource})
			continue
		}
		for _, item := range items {
			out = append(out, held{desc: desc, obj: item})
		}
	}
	return out, names, unread, mentions.counts, failures.Message()
}

const scanConcurrency = 8

type scanned struct {
	counts map[string]int
	errs   map[string]error
}

// scanMentions reads the borrowed kinds and keeps only what they name. The
// objects themselves are freed as each kind finishes: on a cluster with a
// hundred custom kinds, holding all of them at once is where the memory goes.
// They are read at once because one at a time is a minute rather than a second.
func scanMentions(
	ctx context.Context, lister Lister, descs []api.ResourceDescriptor,
) (scanned, []target) {
	wanted := []api.ResourceDescriptor{}
	for _, desc := range descs {
		if borrowed(desc) {
			wanted = append(wanted, desc)
		}
	}
	out := scanned{counts: map[string]int{}, errs: map[string]error{}}
	refused := []target{}
	if len(wanted) == 0 {
		return out, refused
	}
	var lock sync.Mutex
	var group sync.WaitGroup
	slots := make(chan struct{}, scanConcurrency)
	for _, desc := range wanted {
		group.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			items, err := lister.Scan(ctx, desc)
			counted := countMentions(items)
			lock.Lock()
			defer lock.Unlock()
			if err != nil {
				out.errs[descKey(desc)] = err
				refused = append(refused, target{group: desc.Group, resource: desc.Resource})
				return
			}
			for name, seen := range counted {
				out.counts[name] += seen
			}
		})
	}
	group.Wait()
	return out, refused
}

func countMentions(items []*unstructured.Unstructured) map[string]int {
	out := map[string]int{}
	for _, obj := range items {
		here := map[string]bool{}
		gatherStrings(obj.Object, here)
		delete(here, obj.GetName())
		for name := range here {
			out[name]++
		}
	}
	return out
}

// borrowed says this kind is read for the one audit and then let go. Nothing
// browses a custom resource through the audit, so nothing needs its cache kept.
func borrowed(desc api.ResourceDescriptor) bool {
	return desc.Category == customResources
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
	if kind == cronKind {
		return []string{specField, "jobTemplate", specField, "template", specField}
	}
	return []string{specField, "template", specField}
}
