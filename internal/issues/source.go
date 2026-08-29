package issues

import (
	"context"
	"slices"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/listerr"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const maxOwnerDepth = 4

const (
	kindPod                   = "Pod"
	kindDeployment            = "Deployment"
	kindReplicaSet            = "ReplicaSet"
	kindStatefulSet           = "StatefulSet"
	kindDaemonSet             = "DaemonSet"
	kindJob                   = "Job"
	kindReplicationController = "ReplicationController"
	kindCronJob               = "CronJob"
)

const (
	appsGroup        = "apps"
	batchGroup       = "batch"
	autoscalingGroup = "autoscaling"
	argoGroup        = "argoproj.io"
)

const fluxSuffix = ".toolkit.fluxcd.io"

type resourceRef struct {
	group    string
	resource string
}

var coreResources = map[resourceRef]bool{
	{group: "", resource: "pods"}:                                          true,
	{group: "", resource: "nodes"}:                                         true,
	{group: "", resource: "namespaces"}:                                    true,
	{group: "", resource: "persistentvolumeclaims"}:                        true,
	{group: "apiregistration.k8s.io", resource: "apiservices"}:             true,
	{group: "apiextensions.k8s.io", resource: "customresourcedefinitions"}: true,
	{group: "", resource: "services"}:                                      true,
	{group: "", resource: "endpoints"}:                                     true,
	{group: "", resource: "replicationcontrollers"}:                        true,
	{group: appsGroup, resource: "deployments"}:                            true,
	{group: appsGroup, resource: "replicasets"}:                            true,
	{group: appsGroup, resource: "statefulsets"}:                           true,
	{group: appsGroup, resource: "daemonsets"}:                             true,
	{group: batchGroup, resource: "jobs"}:                                  true,
	{group: batchGroup, resource: "cronjobs"}:                              true,
	{group: autoscalingGroup, resource: "horizontalpodautoscalers"}:        true,
	{group: argoGroup, resource: "applications"}:                           true,
}

var fluxResources = []string{
	"kustomizations",
	"helmreleases",
	"gitrepositories",
	"helmrepositories",
	"ocirepositories",
	"buckets",
	"helmcharts",
}

type object struct {
	obj  *unstructured.Unstructured
	desc api.ResourceDescriptor
}

func (item object) ref() api.ObjectRef {
	return api.ObjectRef{
		Group:     item.desc.Group,
		Version:   item.desc.Version,
		Resource:  item.desc.Resource,
		Namespace: item.obj.GetNamespace(),
		Name:      item.obj.GetName(),
	}
}

func (item object) uid() string {
	return string(item.obj.GetUID())
}

type snapshot struct {
	byUID    map[string]object
	byKind   map[string][]object
	byOwner  map[string][]object
	failures *listerr.Collector
}

func newSnapshot() *snapshot {
	return &snapshot{
		byUID:    map[string]object{},
		byKind:   map[string][]object{},
		byOwner:  map[string][]object{},
		failures: listerr.New(),
	}
}

func (snap *snapshot) of(group, kind string) []object {
	return snap.byKind[group+"/"+kind]
}

func (snap *snapshot) owner(item object) object {
	current := item
	for range maxOwnerDepth {
		next, ok := snap.controllerOf(current)
		if !ok {
			return current
		}
		current = next
	}
	return current
}

func controllerUID(obj *unstructured.Unstructured) string {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		return string(ref.UID)
	}
	return ""
}

func (snap *snapshot) children(uid string) []object {
	return snap.byOwner[uid]
}

func (snap *snapshot) controllerOf(item object) (object, bool) {
	refs := item.obj.GetOwnerReferences()
	for _, ref := range refs {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		owner, ok := snap.byUID[string(ref.UID)]
		if !ok {
			return object{}, false
		}
		return owner, true
	}
	return object{}, false
}

func wanted(descs map[string]api.ResourceDescriptor) []api.ResourceDescriptor {
	out := []api.ResourceDescriptor{}
	for _, desc := range descs {
		if !collected(desc) {
			continue
		}
		out = append(out, desc)
	}
	return out
}

func collected(desc api.ResourceDescriptor) bool {
	if coreResources[resourceRef{group: desc.Group, resource: desc.Resource}] {
		return true
	}
	return fluxKind(desc)
}

func fluxKind(desc api.ResourceDescriptor) bool {
	if !isFluxGroup(desc.Group) {
		return false
	}
	return slices.Contains(fluxResources, desc.Resource)
}

func isFluxGroup(group string) bool {
	return len(group) > len(fluxSuffix) && group[len(group)-len(fluxSuffix):] == fluxSuffix
}

func collect(ctx context.Context, lister Lister, descs map[string]api.ResourceDescriptor, limits Limits) *snapshot {
	types := wanted(descs)
	types = append(types, alsoCached(lister, types, limits)...)
	snap := newSnapshot()
	for _, batch := range read(ctx, lister, types, snap.failures, limits) {
		snap.hold(batch.desc, batch.items)
	}
	return snap
}

type batch struct {
	desc  api.ResourceDescriptor
	items []*unstructured.Unstructured
}

func read(
	ctx context.Context,
	lister Lister,
	types []api.ResourceDescriptor,
	failures *listerr.Collector,
	limits Limits,
) []batch {
	out := make([]batch, len(types))
	slots := make(chan struct{}, limits.Readers)
	var wg sync.WaitGroup
	for index, desc := range types {
		wg.Add(1)
		go safe.Run("reading "+desc.Kind, func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
			items, err := lister.Lease(ctx, desc)
			failures.Record(gvr.GroupResource().String(), err)
			if err != nil {
				return
			}
			out[index] = batch{desc: desc, items: items}
		})
	}
	wg.Wait()
	return out
}

func alsoCached(lister Lister, already []api.ResourceDescriptor, limits Limits) []api.ResourceDescriptor {
	seen := map[string]bool{}
	for _, desc := range already {
		seen[keyOf(desc)] = true
	}
	out := []api.ResourceDescriptor{}
	for _, desc := range lister.Cached() {
		if seen[keyOf(desc)] {
			continue
		}
		seen[keyOf(desc)] = true
		out = append(out, desc)
	}
	slices.SortFunc(out, func(left, right api.ResourceDescriptor) int {
		return strings.Compare(keyOf(left), keyOf(right))
	})
	if len(out) > limits.Fallback {
		return out[:limits.Fallback]
	}
	return out
}

func keyOf(desc api.ResourceDescriptor) string {
	return desc.Group + "/" + desc.Version + "/" + desc.Resource
}

func (snap *snapshot) hold(desc api.ResourceDescriptor, found []*unstructured.Unstructured) {
	key := desc.Group + "/" + desc.Kind
	for _, item := range found {
		entry := object{obj: item, desc: desc}
		snap.byUID[entry.uid()] = entry
		snap.byKind[key] = append(snap.byKind[key], entry)
		owner := controllerUID(item)
		if owner == "" {
			continue
		}
		snap.byOwner[owner] = append(snap.byOwner[owner], entry)
	}
}
