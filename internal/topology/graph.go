package topology

import (
	"context"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/listerr"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	categoryWorkload   = "workload"
	categoryPod        = "pod"
	categoryService    = "service"
	categoryIngress    = "ingress"
	categoryConfig     = "config"
	categoryAutoscaler = "autoscaler"
	categoryNamespace  = "namespace"
)

const (
	edgeOwns       = "owns"
	edgeRoutes     = "routes"
	edgeConfigures = "configures"
	edgeScales     = "scales"
)

const (
	kindPod                   = "Pod"
	kindDeployment            = "Deployment"
	kindReplicaSet            = "ReplicaSet"
	kindReplicationController = "ReplicationController"
	kindStatefulSet           = "StatefulSet"
	kindDaemonSet             = "DaemonSet"
	kindJob                   = "Job"
	kindCronJob               = "CronJob"
	kindService               = "Service"
	kindConfigMap             = "ConfigMap"
	kindSecret                = "Secret"
	kindNamespace             = "Namespace"
)

const (
	fieldSpec     = "spec"
	fieldTemplate = "template"
)

const (
	readyTrue     = "True"
	readyFalse    = "False"
	readyUnknown  = "Unknown"
	statusMissing = "NotFound"
)

var listedResources = map[string]string{
	"/pods":                                categoryPod,
	"/services":                            categoryService,
	"/replicationcontrollers":              categoryWorkload,
	"apps/deployments":                     categoryWorkload,
	"apps/replicasets":                     categoryWorkload,
	"apps/statefulsets":                    categoryWorkload,
	"apps/daemonsets":                      categoryWorkload,
	"batch/jobs":                           categoryWorkload,
	"batch/cronjobs":                       categoryWorkload,
	"networking.k8s.io/ingresses":          categoryIngress,
	"autoscaling/horizontalpodautoscalers": categoryAutoscaler,
}

type Lister interface {
	List(ctx context.Context, desc api.ResourceDescriptor) ([]*unstructured.Unstructured, error)
	Warm(ctx context.Context, descs []api.ResourceDescriptor)
}

type Request struct {
	Namespace string
	Root      api.ObjectRef
	Expanded  []string
}

type object struct {
	node   api.GraphNode
	raw    *unstructured.Unstructured
	labels map[string]string
}

type builder struct {
	byKind     map[string]api.ResourceDescriptor
	kindFor    map[string]string
	objects    map[string]*object
	identity   map[string]string
	podsByNs   map[string][]string
	podsByPair map[string]map[string][]string
	controller map[string]string
	edges      map[string]api.GraphEdge
	failures   *listerr.Collector
	namespaces map[string]bool
}

func Build(ctx context.Context, lister Lister, descs map[string]api.ResourceDescriptor, req Request) api.Graph {
	build := &builder{
		byKind:     indexByKind(descs),
		kindFor:    indexKinds(descs),
		objects:    map[string]*object{},
		identity:   map[string]string{},
		podsByNs:   map[string][]string{},
		podsByPair: map[string]map[string][]string{},
		controller: map[string]string{},
		edges:      map[string]api.GraphEdge{},
		failures:   listerr.New(),
		namespaces: map[string]bool{},
	}
	needed := []api.ResourceDescriptor{}
	for _, desc := range descs {
		if categoryFor(desc) == "" {
			continue
		}
		needed = append(needed, desc)
	}
	lister.Warm(ctx, needed)
	for _, desc := range needed {
		build.collect(ctx, lister, desc, req.Namespace)
	}
	build.link()
	return build.graph(req)
}

func categoryFor(desc api.ResourceDescriptor) string {
	return listedResources[desc.Group+"/"+desc.Resource]
}

func indexByKind(descs map[string]api.ResourceDescriptor) map[string]api.ResourceDescriptor {
	byKind := map[string]api.ResourceDescriptor{}
	for _, desc := range descs {
		byKind[desc.Group+"/"+desc.Kind] = desc
	}
	return byKind
}

func indexKinds(descs map[string]api.ResourceDescriptor) map[string]string {
	kinds := map[string]string{}
	for _, desc := range descs {
		kinds[desc.Group+"/"+desc.Resource] = desc.Kind
	}
	return kinds
}

func identityKey(group, kind, namespace, name string) string {
	return group + "/" + kind + "/" + namespace + "/" + name
}

func (b *builder) collect(ctx context.Context, lister Lister, desc api.ResourceDescriptor, namespace string) {
	gvr := schema.GroupVersionResource{Group: desc.Group, Version: desc.Version, Resource: desc.Resource}
	items, err := lister.List(ctx, desc)
	b.failures.Record(gvr.GroupResource().String(), err)
	if err != nil {
		return
	}
	for _, item := range items {
		if namespace != "" && item.GetNamespace() != namespace {
			continue
		}
		b.add(item, desc)
	}
}

func (b *builder) add(obj *unstructured.Unstructured, desc api.ResourceDescriptor) {
	id := string(obj.GetUID())
	if id == "" {
		return
	}
	category := categoryFor(desc)
	b.objects[id] = &object{
		node: api.GraphNode{
			ID:        id,
			Kind:      desc.Kind,
			Group:     desc.Group,
			Version:   desc.Version,
			Resource:  desc.Resource,
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			Status:    statusOf(obj, desc.Kind),
			Ready:     readyOf(obj, desc.Kind),
			Category:  category,
		},
		raw: obj,
	}
	b.identity[identityKey(desc.Group, desc.Kind, obj.GetNamespace(), obj.GetName())] = id
	b.namespaces[obj.GetNamespace()] = true
	if category == categoryPod {
		b.indexPod(id, obj)
	}
}

func (b *builder) indexPod(id string, obj *unstructured.Unstructured) {
	namespace := obj.GetNamespace()
	labels := obj.GetLabels()
	b.objects[id].labels = labels
	b.podsByNs[namespace] = append(b.podsByNs[namespace], id)
	byPair, seen := b.podsByPair[namespace]
	if !seen {
		byPair = map[string][]string{}
		b.podsByPair[namespace] = byPair
	}
	for key, value := range labels {
		pair := key + "=" + value
		byPair[pair] = append(byPair[pair], id)
	}
}

func (b *builder) ensure(group, kind, namespace, name, category, status string) string {
	desc := b.byKind[group+"/"+kind]
	if desc.Resource != "" && !desc.Namespaced {
		namespace = ""
	}
	key := identityKey(group, kind, namespace, name)
	found, ok := b.identity[key]
	if ok {
		return found
	}
	b.objects[key] = &object{node: api.GraphNode{
		ID:        key,
		Kind:      kind,
		Group:     group,
		Version:   desc.Version,
		Resource:  desc.Resource,
		Name:      name,
		Namespace: namespace,
		Status:    status,
		Ready:     readyForPlaceholder(status),
		Category:  category,
	}}
	b.identity[key] = key
	b.namespaces[namespace] = true
	return key
}

func (b *builder) ensureOwner(owner metav1.OwnerReference, namespace string) string {
	id := string(owner.UID)
	_, ok := b.objects[id]
	if ok {
		return id
	}
	group := groupOf(owner.APIVersion)
	desc := b.byKind[group+"/"+owner.Kind]
	if desc.Resource != "" && !desc.Namespaced {
		namespace = ""
	}
	b.objects[id] = &object{node: api.GraphNode{
		ID:        id,
		Kind:      owner.Kind,
		Group:     group,
		Version:   versionOf(owner.APIVersion),
		Resource:  desc.Resource,
		Name:      owner.Name,
		Namespace: namespace,
		Status:    "",
		Ready:     readyUnknown,
		Category:  categoryWorkload,
	}}
	b.identity[identityKey(group, owner.Kind, namespace, owner.Name)] = id
	b.namespaces[namespace] = true
	return id
}

func (b *builder) addEdge(from, to, kind string) {
	if from == "" || to == "" || from == to {
		return
	}
	b.edges[from+"|"+to+"|"+kind] = api.GraphEdge{From: from, To: to, Kind: kind}
}

func groupOf(apiVersion string) string {
	before, _, found := strings.Cut(apiVersion, "/")
	if !found {
		return ""
	}
	return before
}

func versionOf(apiVersion string) string {
	_, after, found := strings.Cut(apiVersion, "/")
	if !found {
		return apiVersion
	}
	return after
}

func readyForPlaceholder(status string) string {
	if status == statusMissing {
		return readyFalse
	}
	return readyUnknown
}

func readyOf(obj *unstructured.Unstructured, kind string) string {
	if notReady(obj, kind) {
		return readyFalse
	}
	if settled(obj, kind) {
		return readyTrue
	}
	return readyUnknown
}

func settled(obj *unstructured.Unstructured, kind string) bool {
	switch kind {
	case kindPod, kindDeployment, kindStatefulSet, kindReplicaSet, kindReplicationController, kindDaemonSet, kindJob:
		return true
	default:
		status, _ := unstr.Ready(obj)
		return status == readyTrue
	}
}

func notReady(obj *unstructured.Unstructured, kind string) bool {
	switch kind {
	case kindPod:
		return podNotReady(obj)
	case kindDeployment, kindStatefulSet, kindReplicaSet, kindReplicationController:
		return unstr.Int(obj, "status", "readyReplicas") < wantedReplicas(obj)
	case kindDaemonSet:
		return unstr.Int(obj, "status", "numberReady") < unstr.Int(obj, "status", "desiredNumberScheduled")
	case kindJob:
		return conditionTrue(obj, "Failed")
	default:
		status, _ := unstr.Ready(obj)
		if status == "" {
			return false
		}
		return status != readyTrue
	}
}

func podNotReady(obj *unstructured.Unstructured) bool {
	phase := unstr.String(obj, "status", "phase")
	if phase == "Succeeded" {
		return false
	}
	if phase == "Failed" {
		return true
	}
	status, _ := unstr.Ready(obj)
	return status != readyTrue
}

func conditionTrue(obj *unstructured.Unstructured, name string) bool {
	for _, raw := range unstr.Slice(obj, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if unstr.At(entry, "type") != name {
			continue
		}
		return unstr.At(entry, "status") == readyTrue
	}
	return false
}

func statusOf(obj *unstructured.Unstructured, kind string) string {
	switch kind {
	case kindPod:
		return unstr.String(obj, "status", "phase")
	case kindDeployment, kindStatefulSet, kindReplicaSet, kindReplicationController:
		return replicaSummary(unstr.Int(obj, "status", "readyReplicas"), wantedReplicas(obj))
	case kindDaemonSet:
		return replicaSummary(
			unstr.Int(obj, "status", "numberReady"),
			unstr.Int(obj, "status", "desiredNumberScheduled"),
		)
	default:
		return unstr.ReadySummary(obj)
	}
}

func wantedReplicas(obj *unstructured.Unstructured) int64 {
	wanted, found, err := unstructured.NestedInt64(obj.Object, fieldSpec, "replicas")
	if err != nil {
		return 1
	}
	if !found {
		return 1
	}
	return wanted
}

func replicaSummary(ready, wanted int64) string {
	return strconv.FormatInt(ready, 10) + "/" + strconv.FormatInt(wanted, 10)
}
